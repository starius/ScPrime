package wallet

import (
	"context"
	"fmt"
	"time"

	"gitlab.com/NebulousLabs/errors"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
	"gitlab.com/scpcorp/spf-transporter"
	"gitlab.com/scpcorp/spf-transporter/common"
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
	return w.customBuildUnsignedBatchTransaction(coinOutputs, fundOutputs, fundbOutputs, buildSpfTxParameters{})
}

type buildSpfTxParameters struct {
	sendFrom        *types.UnlockHash
	onlySpfxRegular bool
}

func (w *Wallet) customBuildUnsignedBatchTransaction(coinOutputs []types.SiacoinOutput, fundOutputs []types.SiafundOutput, fundbOutputs []types.SiafundOutput, spfParams buildSpfTxParameters) (modules.TransactionBuilder, error) {
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
		spfAmount := types.SpfAmount{Amount: totalFundCost, Type: types.SpfA}
		err = txnBuilder.CustomFundSiafunds(spfAmount, spfParams.sendFrom, spfParams.onlySpfxRegular)
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
		spfAmount := types.SpfAmount{Amount: totalFundCost, Type: types.SpfB}
		err = txnBuilder.CustomFundSiafunds(spfAmount, spfParams.sendFrom, spfParams.onlySpfxRegular)
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
	txns, err = w.buildAndSignTxnSet(coinOutputs, fundOutputs, fundbOutputs, buildSpfTxParameters{}, nil)
	if err != nil {
		return nil, err
	}
	if err := w.broadcastTxnSet(txns); err != nil {
		return nil, err
	}
	w.logSuccessfulBroadcast(coinOutputs, fundOutputs, fundbOutputs, txns)
	return txns, nil
}

func (w *Wallet) fetchSiafundBalances(t types.SpfType) (map[types.UnlockHash]types.Currency, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Ensure durability of reported balances.
	if err := w.syncDB(); err != nil {
		return nil, err
	}

	needb := (t == types.SpfB)
	balances := make(map[types.UnlockHash]types.Currency)
	if err := dbForEachSiafundOutput(w.dbTx, func(sfoid types.SiafundOutputID, sfo types.SiafundOutput) {
		isb, err := w.cs.IsSiafundBOutput(sfoid)
		if err != nil {
			return
		}
		if isb != needb {
			return
		}
		sum := balances[sfo.UnlockHash]
		sum = sum.Add(sfo.Value)
		balances[sfo.UnlockHash] = sum
	}); err != nil {
		return nil, err
	}
	return balances, nil
}

func (w *Wallet) SiafundTransportAllowance(t types.SpfType) (*types.SpfTransportAllowance, error) {
	if w.transporterClient == nil {
		return nil, errors.New("transporter is disabled")
	}
	w.mu.RLock()
	unlocked := w.unlocked
	w.mu.RUnlock()
	if !unlocked {
		w.log.Println("Attempt to view SPF transport allowance has failed - wallet is locked")
		return nil, modules.ErrLockedWallet
	}

	ctx := context.Background()
	balances, err := w.fetchSiafundBalances(t)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch SPF balances: %w", err)
	}
	walletAddresses, err := w.AllAddresses()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch wallet addresses: %w", err)
	}
	var regularBalance, preminedBalance types.Currency
	walletPremined := make(map[types.UnlockHash]bool)
	w.mu.RLock()
	for _, addr := range walletAddresses {
		if _, isPremined := w.spfxPreminedAddrs[addr]; isPremined {
			walletPremined[addr] = true
		}
	}
	for uh, amount := range balances {
		if _, isPremined := w.spfxPreminedAddrs[uh]; isPremined {
			preminedBalance = preminedBalance.Add(amount)
		} else {
			regularBalance = regularBalance.Add(amount)
		}
	}
	w.mu.RUnlock()
	totalBalance := preminedBalance.Add(regularBalance)
	allowanceReq := &transporter.CheckAllowanceRequest{}
	if len(walletPremined) != 0 {
		premined := make([]types.UnlockHash, 0, len(walletPremined))
		for wp := range walletPremined {
			premined = append(premined, wp)
		}
		allowanceReq.PreminedUnlockHashes = premined
	}
	allowanceResp, err := w.transporterClient.CheckAllowance(ctx, allowanceReq)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch allowance from transporter: %w", err)
	}
	// Handle premined part of allowance.
	preminedAllowance := make(map[string]types.SpfTransportTypeAllowance)
	for uhStr, p := range allowanceResp.Premined {
		var uh types.UnlockHash
		if err := uh.LoadString(uhStr); err != nil {
			return nil, fmt.Errorf("failed to parse UnlockHash from transporter: %w", err)
		}
		if _, ok := walletPremined[uh]; !ok {
			// Ensure we have this UnlockHash in our wallet.
			continue
		}
		preminedAllowance[uhStr] = types.SpfTransportTypeAllowance{
			MaxAllowed:   types.MinCurrency(p.Amount, totalBalance),
			WaitTime:     p.WaitEstimate,
			PotentialMax: p.Amount,
		}
	}
	// Handle regular.
	maxAllowed := allowanceResp.Regular.Amount
	waitTimeDiff := time.Duration(0)
	if regularBalance.Cmp(allowanceResp.Regular.Amount) < 0 {
		// Wallet balance < allowed amount from transporter.
		maxAllowed := regularBalance
		amountDiff := allowanceResp.Regular.Amount.Sub(maxAllowed)
		// TODO: avoid doing these calculations on spd side.
		// We need some way to get wait estimates for exact (not just max)
		// amounts from transporter.
		waitTimeDiff = spfxEmissionTime(amountDiff)
	}
	regular := types.SpfTransportTypeAllowance{
		MaxAllowed:   maxAllowed,
		WaitTime:     allowanceResp.Regular.WaitEstimate - waitTimeDiff,
		PotentialMax: allowanceResp.Regular.Amount,
	}
	// Airdrop is always skipped (not implemented).
	return &types.SpfTransportAllowance{
		Premined: preminedAllowance,
		Regular:  regular,
	}, nil
}

func (w *Wallet) SiafundTransportHistory() ([]types.SpfTransport, error) {
	if w.transporterClient == nil {
		return nil, errors.New("transporter is disabled")
	}
	w.mu.RLock()
	unlocked := w.unlocked
	w.mu.RUnlock()
	if !unlocked {
		w.log.Println("Attempt to view SPF transport history has failed - wallet is locked")
		return nil, modules.ErrLockedWallet
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	set, err := dbGetAllSpfTransports(w.dbTx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch transports from database: %w", err)
	}
	return set, nil
}

func (w *Wallet) SiafundTransportSend(spfAmount types.SpfAmount, t types.SpfTransportType, preminedUnlockHash *types.UnlockHash, solanaAddr types.SolanaAddress) (time.Duration, *types.Currency, error) {
	if w.transporterClient == nil {
		return 0, nil, errors.New("transporter is disabled")
	}
	w.mu.RLock()
	unlocked := w.unlocked
	w.mu.RUnlock()
	if !unlocked {
		w.log.Println("Attempt to transport SPF has failed - wallet is locked")
		return 0, nil, modules.ErrLockedWallet
	}

	if t == types.Airdrop {
		return 0, nil, errors.New("Airdrop SPF transport type is not supported")
	}
	ctx := context.Background()
	// Start with sanity checks.
	if t == types.Premined && preminedUnlockHash == nil {
		return 0, nil, errors.New("must provide premined unlock hash when type is premined")
	}
	if t != types.Premined && preminedUnlockHash != nil {
		return 0, nil, errors.New("premined unlock hash must be empty when type is not premined")
	}

	// Check allowance.
	allowance, err := w.SiafundTransportAllowance(spfAmount.Type)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to fetch allowance: %w", err)
	}
	if err := allowance.ApplyTo(spfAmount, t, preminedUnlockHash); err != nil {
		return 0, nil, fmt.Errorf("not allowed: %w. no coins were burnts, send canceled.", err)
	}

	// Check Solana address.
	checkSolanaResp, err := w.transporterClient.CheckSolanaAddress(ctx, &transporter.CheckSolanaAddressRequest{
		SolanaAddress: common.SolanaAddress(solanaAddr),
		Amount:        spfAmount.Amount,
	})
	if err != nil {
		return 0, nil, fmt.Errorf("solana address check failed: %w. please double check. no coins were burnt, send canceled.", err)
	}
	localTime := types.CurrentTimestamp().ToStdTime()
	const maxAcceptableTimeGap = 30 * time.Minute
	if localTime.Sub(checkSolanaResp.CurrentTime).Abs() > maxAcceptableTimeGap {
		return 0, nil, fmt.Errorf("please fix your clock before doing any sends, time diff is critical %s local vs %s remote. no coins were burnt, send canceled.", localTime.String(), checkSolanaResp.CurrentTime.String())
	}

	// Build SPF burn transaction (and parents in case we need to build proper SPF inputs first).
	var fundbOutputs, fundaOutputs []types.SiafundOutput
	fundOutput := types.SiafundOutput{
		Value:      spfAmount.Amount,
		UnlockHash: types.BurnAddressUnlockHash,
	}
	if spfAmount.Type == types.SpfA {
		fundaOutputs = append(fundaOutputs, fundOutput)
	} else {
		fundbOutputs = append(fundbOutputs, fundOutput)
	}
	spfParams := buildSpfTxParameters{
		onlySpfxRegular: (t == types.Regular),
		sendFrom:        preminedUnlockHash,
	}
	arbitraryData := common.PutSolanaAddress(common.SolanaAddress(solanaAddr))
	txnSet, err := w.buildAndSignTxnSet(nil, fundaOutputs, fundbOutputs, spfParams, arbitraryData)
	if err != nil {
		return 0, nil, err
	}
	burnTx := txnSet[len(txnSet)-1]
	burnID := burnTx.ID()
	// Sanity check - ensure tx has only one SPF input.
	if t == types.Premined && len(burnTx.SiafundInputs) != 1 {
		return 0, nil, fmt.Errorf("sanity check has failed: expect only 1 SiafundInput for Premined transports, got %d", len(burnTx.SiafundInputs))
	}
	// Sanity check - ensure SPF input is from requested unlock hash.
	sentFrom := burnTx.SiafundInputs[0].UnlockConditions.UnlockHash()
	if t == types.Premined && sentFrom != *preminedUnlockHash {
		return 0, nil, fmt.Errorf("sanity check has failed: got unlock hash %s, expect %s", sentFrom.String(), preminedUnlockHash.String())
	}

	// Save transactions and create a new SPF transport record.
	if err := w.putSpfBurn(burnID, burnTx); err != nil {
		return 0, nil, fmt.Errorf("failed to save transactions before broadcasting: %w", err)
	}
	st := types.SpfTransport{
		BurnID: burnID,
		SpfTransportRecord: types.SpfTransportRecord{
			Status:  types.BurnCreated,
			Amount:  spfAmount.Amount,
			Created: types.CurrentTimestamp(),
		},
	}
	if err := w.putSpfTransport(st); err != nil {
		return 0, nil, fmt.Errorf("failed to create transport record before broadcasting: %w", err)
	}

	// Broadcast transactions.
	if err := w.broadcastTxnSet(txnSet); err != nil {
		return 0, nil, err
	}
	w.logSuccessfulBroadcast(nil, fundaOutputs, fundbOutputs, txnSet)
	st.Status = types.BurnBroadcasted
	if err := w.putSpfTransport(st); err != nil {
		return 0, nil, fmt.Errorf("failed to update transport record after broadcasting: %w", err)
	}

	// Submit burn to transporter.
	const submitRetries = 6
	const submitRetryInterval = time.Second * 10
	resp := &transporter.SubmitScpTxResponse{}
	for i := 0; i < submitRetries; i++ {
		resp, err = w.transporterClient.SubmitScpTx(ctx, &transporter.SubmitScpTxRequest{
			Transaction: burnTx,
		})
		if err == nil {
			break
		}
		w.log.Printf("Failed to submit SCP tx: %v", err)
		time.Sleep(submitRetryInterval)
	}
	if err != nil {
		return 0, nil, fmt.Errorf("failed to submit transaction to transporter: %w", err)
	}

	// Update the record status.
	st.Status = types.SubmittedToTransporter
	if err := w.putSpfTransport(st); err != nil {
		return 0, nil, fmt.Errorf("failed to update transport record after submitting: %w", err)
	}

	return resp.WaitTimeEstimate, resp.SpfAmountAhead, nil
}

func (w *Wallet) logSuccessfulBroadcast(coinOutputs []types.SiacoinOutput, fundOutputs []types.SiafundOutput, fundbOutputs []types.SiafundOutput, txnSet []types.Transaction) {
	var outputList string
	for _, coinOutput := range coinOutputs {
		outputList = outputList + "\n\tAddress: " + coinOutput.UnlockHash.String() + "\n\tValue: " + coinOutput.Value.HumanString() + "\n"
	}
	for _, fundOutput := range fundOutputs {
		fmtSpf := fmt.Sprintf("%14v SPF", fundOutput.Value)
		outputList = outputList + "\n\tAddress: " + fundOutput.UnlockHash.String() + "\n\tValue: " + fmtSpf + "\n"
	}
	txn := txnSet[len(txnSet)-1]
	tPoolFee := types.NewCurrency64(0)
	for _, minerFee := range txn.MinerFees {
		tPoolFee = tPoolFee.Add(minerFee)
	}
	w.log.Printf("Successfully broadcast transaction with id %v, fee %v, and the following outputs: %v", txnSet[len(txnSet)-1].ID(), tPoolFee.HumanString(), outputList)
}

func (w *Wallet) broadcastTxnSet(txnSet []types.Transaction) error {
	if w.deps.Disrupt("SendSiacoinsInterrupted") {
		return errors.New("failed to accept transaction set (SendSiacoinsInterrupted)")
	}
	if w.deps.Disrupt("SendBatchTransaction") {
		return errors.New("failed to accept transaction set (SendBatchTransaction)")
	}
	w.log.Println("Attempting to broadcast a batch transaction over the network")
	if err := w.tpool.AcceptTransactionSet(txnSet); err != nil {
		w.log.Println("Attempt to send coins has failed - transaction pool rejected transaction:", err)
		return build.ExtendErr("unable to get transaction accepted", err)
	}
	return nil
}

func (w *Wallet) buildAndSignTxnSet(coinOutputs []types.SiacoinOutput, fundOutputs []types.SiafundOutput, fundbOutputs []types.SiafundOutput, spfParams buildSpfTxParameters, arbitraryData []byte) (txns []types.Transaction, err error) {
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

	txnBuilder, err := w.customBuildUnsignedBatchTransaction(coinOutputs, fundOutputs, fundbOutputs, spfParams)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			txnBuilder.Drop()
		}
	}()
	if arbitraryData != nil {
		txnBuilder.AddArbitraryData(arbitraryData)
	}
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		w.log.Println("Attempt to send transaction has failed - failed to sign transaction:", err)
		return nil, build.ExtendErr("unable to sign transaction", err)
	}
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
