package wallet

import (
	"fmt"

	"gitlab.com/NebulousLabs/errors"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
)

// estimatedTransactionSize is the estimated size of a transaction used to send
// siacoins.
const estimatedTransactionSize = 750

// sortedOutputs is a struct containing a slice of siacoin outputs and their
// corresponding ids. sortedOutputs can be sorted using the sort package.
type sortedOutputs struct {
	ids     []types.SiacoinOutputID
	outputs []types.SiacoinOutput
}

// DustThreshold returns the quantity per byte below which a Currency is
// considered to be Dust.
func (w *Wallet) DustThreshold() (types.Currency, error) {
	if err := w.tg.Add(); err != nil {
		return types.Currency{}, modules.ErrWalletShutdown
	}
	defer w.tg.Done()

	minFee, _ := w.tpool.FeeEstimation()
	return minFee.Mul64(3), nil
}

// ConfirmedBalance returns the balance of the wallet according to all of the
// confirmed transactions.
func (w *Wallet) ConfirmedBalance() (balance modules.ConfirmedBalance, err error) {
	if err := w.tg.Add(); err != nil {
		return modules.ConfirmedBalance{}, modules.ErrWalletShutdown
	}
	defer w.tg.Done()

	// dustThreshold has to be obtained separate from the lock
	dustThreshold, err := w.DustThreshold()
	if err != nil {
		return modules.ConfirmedBalance{}, modules.ErrWalletShutdown
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// ensure durability of reported balance
	if err = w.syncDB(); err != nil {
		return
	}

	dbForEachSiacoinOutput(w.dbTx, func(_ types.SiacoinOutputID, sco types.SiacoinOutput) {
		if sco.Value.Cmp(dustThreshold) > 0 {
			balance.CoinBalance = balance.CoinBalance.Add(sco.Value)
		}
	})

	siafundPool, err := dbGetSiafundPool(w.dbTx)
	if err != nil {
		return
	}
	dbForEachSiafundOutput(w.dbTx, func(sfoid types.SiafundOutputID, sfo types.SiafundOutput) {
		isSiafundBOutput, err := w.cs.IsSiafundBOutput(sfoid)
		if err != nil {
			return
		}
		if isSiafundBOutput {
			balance.FundbBalance = balance.FundbBalance.Add(sfo.Value)
		} else {
			balance.FundBalance = balance.FundBalance.Add(sfo.Value)
		}
		if sfo.ClaimStart.Cmp(siafundPool) > 0 {
			// Skip claims larger than the siafund pool. This should only
			// occur if the siafund pool has not been initialized yet.
			w.log.Debugf("skipping claim with start value %v because siafund pool is only %v", sfo.ClaimStart, siafundPool)
			return
		}
		claim, err := w.cs.SiafundClaim(sfoid)
		if err != nil {
			return
		}
		if isSiafundBOutput {
			balance.ClaimbBalance = balance.ClaimbBalance.Add(claim.ByOwner)
			lostClaim := types.SiafundBLostClaim(claim)
			balance.UnclaimbBalance = balance.UnclaimbBalance.Add(lostClaim)
		} else {
			balance.ClaimBalance = balance.ClaimBalance.Add(claim.ByOwner)
		}
	})
	return
}

// UnconfirmedBalance returns the number of outgoing and incoming siacoins in
// the unconfirmed transaction set. Refund outputs are included in this
// reporting.
func (w *Wallet) UnconfirmedBalance() (outgoingSiacoins types.Currency, incomingSiacoins types.Currency, err error) {
	if err := w.tg.Add(); err != nil {
		return types.ZeroCurrency, types.ZeroCurrency, modules.ErrWalletShutdown
	}
	defer w.tg.Done()

	// dustThreshold has to be obtained separate from the lock
	dustThreshold, err := w.DustThreshold()
	if err != nil {
		return types.ZeroCurrency, types.ZeroCurrency, modules.ErrWalletShutdown
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	for _, upt := range w.unconfirmedProcessedTransactions {
		for _, input := range upt.Inputs {
			if input.FundType == types.SpecifierSiacoinInput && input.WalletAddress {
				outgoingSiacoins = outgoingSiacoins.Add(input.Value)
			}
		}
		for _, output := range upt.Outputs {
			if output.FundType == types.SpecifierSiacoinOutput && output.WalletAddress && output.Value.Cmp(dustThreshold) > 0 {
				incomingSiacoins = incomingSiacoins.Add(output.Value)
			}
		}
	}
	return
}

// SendSiacoins creates a transaction sending 'amount' to 'dest'. The
// transaction is submitted to the transaction pool and is also returned. Fees
// are added to the amount sent.
func (w *Wallet) SendSiacoins(amount types.Currency, dest types.UnlockHash) ([]types.Transaction, error) {
	if err := w.tg.Add(); err != nil {
		err = modules.ErrWalletShutdown
		return nil, err
	}
	defer w.tg.Done()

	_, fee := w.tpool.FeeEstimation()
	fee = fee.Mul64(estimatedTransactionSize)
	return w.managedSendSiacoins(amount, fee, dest)
}

// SendSiacoinsFeeIncluded creates a transaction sending 'amount' to 'dest'. The
// transaction is submitted to the transaction pool and is also returned. Fees
// are subtracted from the amount sent.
func (w *Wallet) SendSiacoinsFeeIncluded(amount types.Currency, dest types.UnlockHash) ([]types.Transaction, error) {
	if err := w.tg.Add(); err != nil {
		err = modules.ErrWalletShutdown
		return nil, err
	}
	defer w.tg.Done()

	_, fee := w.tpool.FeeEstimation()
	fee = fee.Mul64(estimatedTransactionSize)
	// Don't allow sending an amount equal to the fee, as zero spending is not
	// allowed and would error out later.
	if amount.Cmp(fee) <= 0 {
		w.log.Println("Attempt to send coins has failed - not enough to cover fee")
		return nil, errors.AddContext(modules.ErrLowBalance, "not enough coins to cover fee")
	}
	return w.managedSendSiacoins(amount.Sub(fee), fee, dest)
}

// managedSendSiacoins creates a transaction sending 'amount' to 'dest'. The
// transaction is submitted to the transaction pool and is also returned.
func (w *Wallet) managedSendSiacoins(amount, fee types.Currency, dest types.UnlockHash) (txns []types.Transaction, err error) {
	// Check if consensus is synced
	if !w.cs.Synced() || w.deps.Disrupt("UnsyncedConsensus") {
		return nil, errors.New("cannot send scprimecoin until fully synced")
	}

	w.mu.RLock()
	unlocked := w.unlocked
	w.mu.RUnlock()
	if !unlocked {
		w.log.Println("Attempt to send coins has failed - wallet is locked")
		return nil, modules.ErrLockedWallet
	}

	output := types.SiacoinOutput{
		Value:      amount,
		UnlockHash: dest,
	}

	txnBuilder, err := w.StartTransaction()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			txnBuilder.Drop()
		}
	}()
	err = txnBuilder.FundSiacoins(amount.Add(fee))
	if err != nil {
		w.log.Println("Attempt to send coins has failed - failed to fund transaction:", err)
		return nil, build.ExtendErr("unable to fund transaction", err)
	}
	txnBuilder.AddMinerFee(fee)
	txnBuilder.AddSiacoinOutput(output)
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		w.log.Println("Attempt to send coins has failed - failed to sign transaction:", err)
		return nil, build.ExtendErr("unable to sign transaction", err)
	}
	if w.deps.Disrupt("SendSiacoinsInterrupted") {
		return nil, errors.New("failed to accept transaction set (SendSiacoinsInterrupted)")
	}
	err = w.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		w.log.Println("Attempt to send coins has failed - transaction pool rejected transaction:", err)
		return nil, build.ExtendErr("unable to get transaction accepted", err)
	}
	w.log.Println("Submitted a scprimecoin transfer transaction set for value", amount.HumanString(), "with fees", fee.HumanString(), "IDs:")
	for _, txn := range txnSet {
		w.log.Println("\t", txn.ID())
	}
	return txnSet, nil
}

// BuildUnsignedBatchTransaction builds and returns an unsigned transaction.
func (w *Wallet) BuildUnsignedBatchTransaction(coinOutputs []types.SiacoinOutput, fundOutputs []types.SiafundOutput, fundbOutputs []types.SiafundOutput) (modules.TransactionBuilder, error) {
	if len(fundOutputs) > 0 && len(fundbOutputs) > 0 {
		return nil, errors.New("cannot send both spf-a & spf-b in one transaction")
	}
	// Check if consensus is synced
	if !w.cs.Synced() || w.deps.Disrupt("UnsyncedConsensus") {
		return nil, errors.New("cannot build batch transaction until fully synced")
	}
	// Check if wallet is locked
	w.mu.RLock()
	unlocked := w.unlocked
	w.mu.RUnlock()
	if !unlocked {
		w.log.Println("Attempt to send coins has failed - wallet is locked")
		return nil, modules.ErrLockedWallet
	}
	txnBuilder, err := w.StartTransaction()
	defer func() {
		if err != nil {
			txnBuilder.Drop()
		}
	}()
	// Add estimated transaction fee.
	_, estTpoolFee := w.tpool.FeeEstimation()
	tPoolFee := types.NewCurrency64(0)
	if len(coinOutputs) != 0 {
		coinTpoolFee := estTpoolFee.Mul64(2)                                  // We don't want send-to-many transactions to fail.
		coinTpoolFee = coinTpoolFee.Mul64(1000 + 60*uint64(len(coinOutputs))) // Estimated transaction size in bytes
		tPoolFee = tPoolFee.Add(coinTpoolFee)
	}
	if len(fundOutputs) != 0 {
		fundTpoolFee := estTpoolFee.Mul64(5)                                 // use large fee to ensure siafund transactions are selected by minerstopcmd
		fundTpoolFee = fundTpoolFee.Mul64(690 + 60*uint64(len(fundOutputs))) // Estimated transaction size in bytes
		tPoolFee = tPoolFee.Add(fundTpoolFee)
	}
	if len(fundbOutputs) != 0 {
		fundTpoolFee := estTpoolFee.Mul64(5)                                  // use large fee to ensure siafund transactions are selected by minerstopcmd
		fundTpoolFee = fundTpoolFee.Mul64(690 + 60*uint64(len(fundbOutputs))) // Estimated transaction size in bytes
		tPoolFee = tPoolFee.Add(fundTpoolFee)
	}
	txnBuilder.AddMinerFee(tPoolFee)

	// Calculate total cost to wallet.
	//
	// NOTE: we only want to call FundSiacoins and FundSiafunds once; that way,
	// it will (ideally) fund the entire transaction with a single input,
	// instead of many smaller ones.
	totalCoinCost := tPoolFee
	for _, coinOutput := range coinOutputs {
		totalCoinCost = totalCoinCost.Add(coinOutput.Value)
	}
	err = txnBuilder.FundSiacoins(totalCoinCost)
	if err != nil {
		return nil, build.ExtendErr("not enoutgh SCP to fund transaction", err)
	}
	for _, coinOutput := range coinOutputs {
		txnBuilder.AddSiacoinOutput(coinOutput)
	}
	if len(fundOutputs) != 0 {
		totalFundCost := types.NewCurrency64(0)
		for _, fundOutput := range fundOutputs {
			totalFundCost = totalFundCost.Add(fundOutput.Value)
		}
		err = txnBuilder.FundSiafunds(totalFundCost, false)
		if err != nil {
			return nil, build.ExtendErr("not enough SPF to fund transaction", err)
		}
		for _, fundOutput := range fundOutputs {
			txnBuilder.AddSiafundOutput(fundOutput)
		}
	}
	if len(fundbOutputs) != 0 {
		totalFundCost := types.NewCurrency64(0)
		for _, fundbOutput := range fundbOutputs {
			totalFundCost = totalFundCost.Add(fundbOutput.Value)
		}
		err = txnBuilder.FundSiafunds(totalFundCost, true)
		if err != nil {
			return nil, build.ExtendErr("not enough SPF-B to fund transaction", err)
		}
		for _, fundbOutput := range fundbOutputs {
			txnBuilder.AddSiafundOutput(fundbOutput)
		}
	}
	return txnBuilder, nil
}

// SendBatchTransaction creates a transaction that includes the specified
// coin or fund outputs. The transaction is submitted to the transaction
// pool and is also returned.
//
// NOTE: The ScPrime.info blockchain explorer does not currently correctly
// display transactions that contain both SPF and SCP outputs. Specifically,
// the explorer displays all SCP outputs as miner fees when an SPF output
// is specified. Since it is important that the general public has confidence
// in the blockchain explorer the SendBatchTransaction function is artificially
// limited to not allowing a user to define both coin and fund outputs.
// Ideally the blockchain explorer will someday be fixed to correctly display
// these types of transactions. After this happens the check to prevent both coin
// and fund outputs from being supplied can be removed.
func (w *Wallet) SendBatchTransaction(coinOutputs []types.SiacoinOutput, fundOutputs []types.SiafundOutput, fundbOutputs []types.SiafundOutput) (txns []types.Transaction, err error) {
	// TODO: Fix the ScPrime.info blockchain explorer to correctly display
	// transactions with both SPF and SCP outputs. Afterwhich, this check
	// should be removed.
	if (len(coinOutputs) != 0 && len(fundOutputs) != 0) ||
		(len(coinOutputs) != 0 && len(fundbOutputs) != 0) ||
		(len(fundOutputs) != 0 && len(fundbOutputs) != 0) {
		return nil, errors.New("cannot supply different kind of outputs in one transaction")
	}
	if err := w.tg.Add(); err != nil {
		err = modules.ErrWalletShutdown
		return nil, err
	}
	defer w.tg.Done()
	w.log.Println("Beginning call to SendBatchTransaction")

	txnBuilder, err := w.BuildUnsignedBatchTransaction(coinOutputs, fundOutputs, fundbOutputs)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			txnBuilder.Drop()
		}
	}()
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		w.log.Println("Attempt to send transaction has failed - failed to sign transaction:", err)
		return nil, build.ExtendErr("unable to sign transaction", err)
	}
	if w.deps.Disrupt("SendSiacoinsInterrupted") {
		return nil, errors.New("failed to accept transaction set (SendSiacoinsInterrupted)")
	}
	if w.deps.Disrupt("SendBatchTransaction") {
		return nil, errors.New("failed to accept transaction set (SendBatchTransaction)")
	}
	w.log.Println("Attempting to broadcast a batch transaction over the network")
	err = w.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		w.log.Println("Attempt to send coins has failed - transaction pool rejected transaction:", err)
		return nil, build.ExtendErr("unable to get transaction accepted", err)
	}
	// Log the success.
	var outputList string
	for _, coinOutput := range coinOutputs {
		outputList = outputList + "\n\tAddress: " + coinOutput.UnlockHash.String() + "\n\tValue: " + coinOutput.Value.HumanString() + "\n"
	}
	for _, fundOutput := range fundOutputs {
		fmtSpf := fmt.Sprintf("%14v SPF", fundOutput.Value)
		outputList = outputList + "\n\tAddress: " + fundOutput.UnlockHash.String() + "\n\tValue: " + fmtSpf + "\n"
	}
	txn, _ := txnBuilder.View()
	tPoolFee := types.NewCurrency64(0)
	for _, minerFee := range txn.MinerFees {
		tPoolFee = tPoolFee.Add(minerFee)
	}
	w.log.Printf("Successfully broadcast transaction with id %v, fee %v, and the following outputs: %v", txnSet[len(txnSet)-1].ID(), tPoolFee.HumanString(), outputList)
	return txnSet, nil
}

// SendSiacoinsMulti creates a transaction that includes the specified
// outputs. The transaction is submitted to the transaction pool and is also
// returned.
func (w *Wallet) SendSiacoinsMulti(coinOutputs []types.SiacoinOutput) (txns []types.Transaction, err error) {
	return w.SendBatchTransaction(coinOutputs, nil, nil)
}

// SendSiafundsMulti creates a transaction that includes the specified
// outputs. The transaction is submitted to the transaction pool and is also
// returned.
func (w *Wallet) SendSiafundsMulti(fundOutputs []types.SiafundOutput) (txns []types.Transaction, err error) {
	return w.SendBatchTransaction(nil, fundOutputs, nil)
}

// SendSiafunds creates a transaction sending 'amount' to 'dest'. The transaction
// is submitted to the transaction pool and is also returned.
func (w *Wallet) SendSiafunds(amount types.Currency, dest types.UnlockHash) (txns []types.Transaction, err error) {
	var fundOutputs []types.SiafundOutput
	fundOutput := types.SiafundOutput{
		Value:      amount,
		UnlockHash: dest,
	}
	fundOutputs = append(fundOutputs, fundOutput)
	return w.SendBatchTransaction(nil, fundOutputs, nil)
}

// SendSiafundbs creates a transaction sending 'amount' to 'dest'. The transaction
// is submitted to the transaction pool and is also returned.
func (w *Wallet) SendSiafundbs(amount types.Currency, dest types.UnlockHash) (txns []types.Transaction, err error) {
	fundbOutputs := []types.SiafundOutput{
		{
			Value:      amount,
			UnlockHash: dest,
		},
	}
	return w.SendBatchTransaction(nil, nil, fundbOutputs)
}

// Len returns the number of elements in the sortedOutputs struct.
func (so sortedOutputs) Len() int {
	if build.DEBUG && len(so.ids) != len(so.outputs) {
		panic("sortedOutputs object is corrupt")
	}
	return len(so.ids)
}

// Less returns whether element 'i' is less than element 'j'. The currency
// value of each output is used for comparison.
func (so sortedOutputs) Less(i, j int) bool {
	return so.outputs[i].Value.Cmp(so.outputs[j].Value) < 0
}

// Swap swaps two elements in the sortedOutputs set.
func (so sortedOutputs) Swap(i, j int) {
	so.ids[i], so.ids[j] = so.ids[j], so.ids[i]
	so.outputs[i], so.outputs[j] = so.outputs[j], so.outputs[i]
}
