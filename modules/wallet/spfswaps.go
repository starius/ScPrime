package wallet

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
)

//var spd *client.Client

var (
	defaultMinerFee       = types.ScPrimecoinPrecision.Div64(100)
	swapMissingInputs     = errors.New("transaction is missing inputs")
	swapMissingOutputs    = errors.New("transaction is missing outputs")
	swapMissingSignatures = errors.New("transaction is missing counterparty signatures")
)

const (
	//for indication the status of offer
	waitingForYouToAccept          = "waitingForYouToAccept"
	waitingForCounterpartyToAccept = "waitingForCounterpartyToAccept"
	waitingForYouToFinish          = "waitingForYouToFinish"
	waitingForCounterpartyToFinish = "waitingForCounterpartyToFinish"
	swapOfferPending               = "SwapOfferPending"
	swapOfferConfirmed             = "SwapOfferConfirmed"
)

// CreateSwapOffer creates a transaction proposal for exchanging between SCP and SPF
func (w *Wallet) CreateSwapOffer(amountOffered types.Currency, typeOffered types.Specifier, amountAccepted types.Currency, typeAccepted types.Specifier, receiveAddress types.UnlockHash) (modules.SwapOffer, error) {
	//sanity checks
	unlocked, err := w.Unlocked()
	if err != nil {
		return modules.SwapOffer{}, fmt.Errorf("error accessing wallet: %w", err)
	}
	if !unlocked {
		return modules.SwapOffer{}, fmt.Errorf("can not create swap offer as wallet not unlocked")
	}
	if receiveAddress == (types.UnlockHash{}) || receiveAddress == types.BurnAddressUnlockHash {
		uc, err := w.NextAddress()
		if err != nil {
			return modules.SwapOffer{}, fmt.Errorf("error generating wallet address: %w", err)
		}
		receiveAddress = uc.UnlockHash()
	} else if !w.isWalletAddress(receiveAddress) {
		return modules.SwapOffer{}, fmt.Errorf("receive address valid for this wallet is needed")
	}
	if typeOffered == typeAccepted {
		return modules.SwapOffer{}, fmt.Errorf("can not exchange <%v> for <%v> (same type of funds)", fundingName(typeOffered), fundingName(typeAccepted))
	}
	if amountOffered.IsZero() {
		return modules.SwapOffer{}, fmt.Errorf("can not exchange nothing for something")
	}
	if amountAccepted.IsZero() {
		return modules.SwapOffer{}, fmt.Errorf("can not exchange something for nothing")
	}
	return w.createSwap(amountOffered, typeOffered, amountAccepted, typeAccepted, receiveAddress)
}

// AcceptSwapOffer accepts an offered swap transaction by filling in missing amounts and addresses and signing the transaction fields
func (w *Wallet) AcceptSwapOffer(swapOffer modules.SwapOffer, receiveAddress types.UnlockHash) (modules.SwapOffer, error) {
	//sanity checks
	err := checkAccept(swapOffer)
	if err != nil {
		return swapOffer, fmt.Errorf("swap offer not acceptable: %w", err)
	}
	unlocked, err := w.Unlocked()
	if err != nil {
		return swapOffer, fmt.Errorf("error accessing wallet: %w", err)
	}
	if !unlocked {
		return swapOffer, fmt.Errorf("can not create swap offer as wallet not unlocked")
	}
	if !w.isWalletAddress(receiveAddress) {
		return swapOffer, fmt.Errorf("specified receive address does not belong to this wallet")
	}
	if w.isWalletAddress(swapOffer.SCPOutputs[0].UnlockHash) || w.isWalletAddress(swapOffer.SPFOutputs[0].UnlockHash) {
		return swapOffer, fmt.Errorf("can't accept using the wallet the offer was created")
	}

	//make a copy of original
	acceptedOffer := swapOffer
	err = w.acceptSwap(&acceptedOffer, receiveAddress)
	if err != nil {
		return swapOffer, fmt.Errorf("error accepting swap offer: %w", err)
	}
	_, err = w.CheckSwapOffer(acceptedOffer)
	if err != nil {
		return swapOffer, fmt.Errorf("error accepting swap offer: %w", err)
	}
	return acceptedOffer, nil
}

// FinalizeSwapOffer finalizes the offer after it was accepted by counterparty
func (w *Wallet) FinalizeSwapOffer(swapOffer modules.SwapOffer) ([]types.Transaction, error) {
	swapTX := swapOffer
	//determine which signatures are missing
	var haveSCPSignatures bool
	for _, sci := range swapTX.SCPInputs {
		if crypto.Hash(sci.ParentID) == swapTX.Signatures[0].ParentID {
			haveSCPSignatures = true
			break
		}
	}
	var err error
	if haveSCPSignatures {
		err = signSPF(&swapTX, w)
	} else {
		err = signSCP(&swapTX, w)
	}
	if err != nil {
		return []types.Transaction{}, fmt.Errorf("failed to finalize (sign) swap transaction: %w", err)
	}
	txns := []types.Transaction{swapTX.Transaction()}
	return txns, w.tpool.AcceptTransactionSet(txns)
}

// addSCP modifies the swap offer by appending SCP inputs to fill the resulting SCP output and pay the miner fee
func (w *Wallet) addSCP(swap *modules.SwapOffer, amount, minerFee types.Currency) error {
	wug, err := w.UnspentOutputs()
	if err != nil {
		return fmt.Errorf("failed to get unspent outputs: %w", err)
	}
	totalToAdd := amount.Add(minerFee)
	availableOutputs := unspentOutputs{}
	for _, uo := range wug {
		if uo.FundType == types.SpecifierSiacoinOutput {
			//skip sorting and adding if perfect match encountered
			if amount.Equals(uo.Value) {
				//dismiss anything previously gathered
				availableOutputs = unspentOutputs{}
				availableOutputs.Append(uo)
				break
			}
			availableOutputs.Append(uo)
		}
	}
	if availableOutputs.scpOutputs.Len() < 1 {
		return fmt.Errorf("no SCP outputs available")
	}
	availableOutputs.Sort()

	var inputSum types.Currency
	for _, u := range availableOutputs.scpOutputs {
		uc, err := w.UnlockConditions(u.UnlockHash)
		if err != nil {
			return fmt.Errorf("failed to get address %v unlock conditions: %w", u.UnlockHash, err)
		}
		swap.SCPInputs = append(swap.SCPInputs, types.SiacoinInput{
			ParentID:         types.SiacoinOutputID(u.ID),
			UnlockConditions: uc,
		})
		inputSum = inputSum.Add(u.Value)
		if inputSum.Cmp(totalToAdd) >= 0 {
			break
		}
	}

	if inputSum.Cmp(totalToAdd) < 0 {
		return errors.New("insufficient funds")
	}
	//add miner fee
	swap.TransactionFee = minerFee
	// add a change output, if necessary
	if !inputSum.Equals(totalToAdd) {
		//use SPF receiving address for SCP change return too
		swap.SCPOutputs = append(swap.SCPOutputs, types.SiacoinOutput{
			UnlockHash: swap.SPFOutputs[0].UnlockHash,
			Value:      inputSum.Sub(totalToAdd),
		})
	}
	return nil
}

// addSPF modifies the swap offer by appending SPF inputs to fill the resulting SPF output
func (w *Wallet) addSPF(swap *modules.SwapOffer, amount types.Currency, spfType types.Specifier) error {
	wug, err := w.UnspentOutputs()
	if err != nil {
		return fmt.Errorf("failed to get wallet unspent outputs: %w", err)
	}
	availableOutputs := unspentOutputs{}
	for _, uo := range wug {
		//skip sorting and adding if perfect match encountered
		if uo.FundType == spfType && amount.Equals(uo.Value) {
			//dismiss anything previously gathered
			availableOutputs = unspentOutputs{}
			availableOutputs.Append(uo)
			break
		}
		availableOutputs.Append(uo)
	}
	if len(availableOutputs.Outputs(spfType)) < 1 {
		return fmt.Errorf("no %v available", fundingName(spfType))
	}
	availableOutputs.Sort()

	var inputSum types.Currency
	for _, u := range availableOutputs.Outputs(spfType) {
		uc, err := w.UnlockConditions(u.UnlockHash)
		if err != nil {
			return fmt.Errorf("failed to get address %v unlock conditions: %w", u.UnlockHash, err)
		}
		swap.SPFInputs = append(swap.SPFInputs, types.SiafundInput{
			ParentID:         types.SiafundOutputID(u.ID),
			UnlockConditions: uc,
			ClaimUnlockHash:  u.UnlockHash,
		})
		inputSum = inputSum.Add(u.Value)
		if inputSum.Cmp(amount) >= 0 {
			break
		}
	}
	if inputSum.Cmp(amount) < 0 {
		return errors.New("insufficient funds")
	}
	// add a change output, if necessary, use the SCP receive address for that
	if !inputSum.Equals(amount) {
		swap.SPFOutputs = append(swap.SPFOutputs, types.SiafundOutput{
			UnlockHash: swap.SCPOutputs[0].UnlockHash,
			Value:      inputSum.Sub(amount),
		})
	}
	return nil
}

func signSCP(swap *modules.SwapOffer, w *Wallet) error {
	var toSign []crypto.Hash
	for _, sci := range swap.SCPInputs {
		swap.Signatures = append(swap.Signatures, types.TransactionSignature{
			ParentID:       crypto.Hash(sci.ParentID),
			PublicKeyIndex: 0,
			CoveredFields:  types.FullCoveredFields,
		})
		toSign = append(toSign, crypto.Hash(sci.ParentID))
	}
	txn := swap.Transaction()
	err := w.SignTransaction(&txn, toSign)
	swap.Signatures = txn.TransactionSignatures
	return err
}

func signSPF(swap *modules.SwapOffer, w *Wallet) error {
	var toSign []crypto.Hash
	for _, sfi := range swap.SPFInputs {
		swap.Signatures = append(swap.Signatures, types.TransactionSignature{
			ParentID:       crypto.Hash(sfi.ParentID),
			PublicKeyIndex: 0,
			CoveredFields:  types.FullCoveredFields,
		})
		toSign = append(toSign, crypto.Hash(sfi.ParentID))
	}
	txn := swap.Transaction()
	err := w.SignTransaction(&txn, toSign)
	swap.Signatures = txn.TransactionSignatures
	return err
}

// Human naming for type specifier
func fundingName(spec types.Specifier) string {
	switch spec {
	case types.SpecifierSiafundBOutput:
		return "SPF-B"
	case types.SpecifierSiafundOutput:
		return "SPF-A"
	}
	return "SCP"
}

// createSwap creates a new SwapOffer offer swapping the offerAmount for the
// acceptAmount.
func (w *Wallet) createSwap(offerAmount types.Currency, offerType types.Specifier, acceptAmount types.Currency, acceptType types.Specifier, receiveAddr types.UnlockHash) (modules.SwapOffer, error) {
	swap := modules.SwapOffer{
		OfferedFunds:  fundingName(offerType),
		AcceptedFunds: fundingName(acceptType),
	}
	switch offerType {
	case types.SpecifierSiacoinOutput: //SCP
		swap.SCPOutputs = append(swap.SCPOutputs, types.SiacoinOutput{
			Value:      offerAmount,
			UnlockHash: types.UnlockHash{}, // to be filled in by counterparty
		})
		swap.SPFOutputs = append(swap.SPFOutputs, types.SiafundOutput{
			Value:      acceptAmount,
			UnlockHash: receiveAddr,
		})
		// the party that contributes SCP is responsible for paying the miner fee
		if err := w.addSCP(&swap, offerAmount, defaultMinerFee); err != nil {
			return swap, fmt.Errorf("failed to add SCP to swap transaction: %w", err)
		}
	default: //any of SPF types
		swap.SCPOutputs = append(swap.SCPOutputs, types.SiacoinOutput{
			Value:      acceptAmount,
			UnlockHash: receiveAddr,
		})
		swap.SPFOutputs = append(swap.SPFOutputs, types.SiafundOutput{
			Value:      offerAmount,
			UnlockHash: types.UnlockHash{}, // to be filled in by counterparty
		})
		if err := w.addSPF(&swap, offerAmount, offerType); err != nil {
			return swap, fmt.Errorf("failed to add SPF to swap transaction: %w", err)
		}
	}
	return swap, nil
}

// checkAccept checks that the counterparty's swap offer is valid and not accepted yet
func checkAccept(swap modules.SwapOffer) error {
	if len(swap.SCPInputs) == 0 && len(swap.SPFInputs) == 0 {
		return errors.New("transaction has no inputs")
	} else if len(swap.SCPInputs) > 0 && len(swap.SPFInputs) > 0 {
		return errors.New("only one set of inputs should be provided")
	} else if len(swap.SCPOutputs) == 0 && len(swap.SPFOutputs) == 0 {
		return errors.New("transaction has no outputs")
	} else if swap.SCPOutputs[0].UnlockHash == (types.UnlockHash{}) && swap.SPFOutputs[0].UnlockHash == (types.UnlockHash{}) {
		return errors.New("one output address should be left unspecified")
	} else if len(swap.Signatures) > 0 {
		return errors.New("transaction should not have any signatures yet")
	}
	return nil
}

// acceptSwap accepts and signs a swap transaction.
func (w *Wallet) acceptSwap(swap *modules.SwapOffer, receiveAddr types.UnlockHash) error {
	if len(swap.SCPInputs) == 0 {
		//if no SCP inputs in the offer means we put our receive address to receive SPF
		swap.SPFOutputs[0].UnlockHash = receiveAddr
		// and add SCP
		if err := w.addSCP(swap, swap.SCPOutputs[0].Value, defaultMinerFee); err != nil {
			return fmt.Errorf("failed to add SCP inputs: %w", err)
		}
		return signSCP(swap, w)
	}
	//else we put our receive address to receive SCP
	swap.SCPOutputs[0].UnlockHash = receiveAddr
	//and add SPF
	if err := w.addSPF(swap, swap.SPFOutputs[0].Value, swap.AcceptedTypeSpecifier()); err != nil {
		return fmt.Errorf("failed to add SPF inputs: %w", err)
	}
	return signSPF(swap, w)
}

// checkFinish checks that the accepted swap transaction is valid.
func (w *Wallet) checkFinish(swap modules.SwapOffer, theirs bool) error {
	if len(swap.SCPInputs) == 0 || len(swap.SPFInputs) == 0 {
		return swapMissingInputs
	} else if len(swap.SCPOutputs) == 0 || len(swap.SPFOutputs) == 0 {
		return swapMissingOutputs
	} else if swap.SCPOutputs[0].UnlockHash == (types.UnlockHash{}) || swap.SPFOutputs[0].UnlockHash == (types.UnlockHash{}) {
		return errors.New("one or both swap output addresses have been left unspecified")
	} else if len(swap.Signatures) == 0 {
		return swapMissingSignatures
	}

	wag, err := w.AllAddresses()
	if err != nil {
		return fmt.Errorf("failed to get wallet addresses: %w", err)
	}
	belongsToUs := make(map[types.UnlockHash]bool)
	for _, uh := range wag {
		belongsToUs[uh] = true
	}

	var haveSCPSignature bool
	for _, sci := range swap.SCPInputs {
		if crypto.Hash(sci.ParentID) == swap.Signatures[0].ParentID {
			haveSCPSignature = true
			break
		}
	}
	if !theirs && haveSCPSignature || theirs && !haveSCPSignature {
		// all of the SPF inputs should belong to us
		for _, sfi := range swap.SPFInputs {
			if !belongsToUs[sfi.UnlockConditions.UnlockHash()] {
				return errors.New("counterparty added an SPF input that does not belong to us")
			}
		}
		// none of the SCP inputs should belong to us
		for _, sci := range swap.SCPInputs {
			if belongsToUs[sci.UnlockConditions.UnlockHash()] {
				return errors.New("counterparty added an SCP input that belongs to us")
			}
		}
		// all of the SPF change outputs should belong to us
		for _, sfo := range swap.SPFOutputs[1:] {
			if !belongsToUs[sfo.UnlockHash] {
				return errors.New("counterparty added an SPF output that does not belong to us")
			}
		}
		// the SCP output should belong to us
		if !belongsToUs[swap.SCPOutputs[0].UnlockHash] {
			return errors.New("the SCP output address does not belong to us")
		}
	} else {
		// all of the SCP inputs should belong to us
		for _, sci := range swap.SCPInputs {
			if !belongsToUs[sci.UnlockConditions.UnlockHash()] {
				return errors.New("counterparty added an SCP input that does not belong to us")
			}
		}
		// none of the SPF inputs should belong to us
		for _, sfi := range swap.SPFInputs {
			if belongsToUs[sfi.UnlockConditions.UnlockHash()] {
				return errors.New("counterparty added an SPF input that belongs to us")
			}
		}
		// all of the SCP change outputs should belong to us
		for _, sco := range swap.SCPOutputs[1:] {
			if !belongsToUs[sco.UnlockHash] {
				return errors.New("counterparty added an SCP output that does not belong to us")
			}
		}
		// the SPF output should belong to us
		if !belongsToUs[swap.SPFOutputs[0].UnlockHash] {
			return errors.New("the SPF output address does not belong to us")
		}
	}
	return nil
}

// CheckSwapOffer returns a summary of the swap.
func (w *Wallet) CheckSwapOffer(swap modules.SwapOffer) (s modules.SwapSummary, err error) {
	wag, err := w.AllAddresses()
	if err != nil {
		return modules.SwapSummary{}, fmt.Errorf("failed to get wallet addresses: %w", err)
	}
	//See if the check is being done on the initiator or acceptor wallet
	for _, addr := range wag {
		if swap.SCPOutputs[0].UnlockHash == addr {
			s.ReceiveSCP = true
		}
		if swap.SPFOutputs[0].UnlockHash == addr {
			s.ReceiveSPF = true
		}
	}
	if s.ReceiveSCP && s.ReceiveSPF {
		return modules.SwapSummary{}, fmt.Errorf("Invalid swap: can not swap using same wallet")
	}
	//If none of receive address is known means it is potentially an available
	//offer and can be accepted by replacing empty address (burn address) in the
	// outputs with an owned address
	if !s.ReceiveSCP && !s.ReceiveSPF {
		if swap.SCPOutputs[0].UnlockHash == (types.UnlockHash{}) {
			s.ReceiveSCP = true
		}
		if swap.SPFOutputs[0].UnlockHash == (types.UnlockHash{}) {
			s.ReceiveSPF = true
		}
		if s.ReceiveSCP && s.ReceiveSPF {
			return modules.SwapSummary{}, fmt.Errorf("Empty offer")
		}
	}

	s.AmountSCP = swap.SCPOutputs[0].Value
	s.AmountSPF = swap.SPFOutputs[0].Value
	s.MinerFee = swap.TransactionFee
	s.Status = status(swap, w)
	if s.Status == "" {
		return modules.SwapSummary{}, fmt.Errorf("failed to get swap status")
	}
	return
}

// acceptStatus checks if the swap is ready to be accepted and which party needs to accept.
func acceptStatus(swap modules.SwapOffer, w *Wallet) string {
	if err := checkAccept(swap); err != nil {
		return ""
	}
	wag, err := w.AllAddresses()
	if err != nil {
		return ""
	}
	for _, addr := range wag {
		if swap.SCPOutputs[0].UnlockHash == addr || swap.SPFOutputs[0].UnlockHash == addr {
			return waitingForCounterpartyToAccept
		}
	}
	return waitingForYouToAccept
}

// finishStatus checks if the swap is ready to be finished and which party needs to finish.
func finishStatus(swap modules.SwapOffer, w *Wallet) string {
	err := w.checkFinish(swap, false)
	if err == nil {
		return waitingForYouToFinish
	}
	err = w.checkFinish(swap, true)
	if err == nil {
		return waitingForCounterpartyToFinish
	}
	return ""
}

// txnStatus checks if the swap txn is in the txn pool and whether its confirmed.
func txnStatus(swap modules.SwapOffer, w *Wallet) string {
	tx, exists, err := w.Transaction(swap.Transaction().ID())
	if err != nil {
		w.log.Debugf("error checking transaction: %v", err.Error())
	}
	if err != nil || !exists {
		return ""
	}
	if tx.ConfirmationHeight == math.MaxUint64 {
		return swapOfferPending
	}
	return swapOfferConfirmed
}

// status gets the overall status of a swap txn.
func status(swap modules.SwapOffer, w *Wallet) string {
	if status := txnStatus(swap, w); status != "" {
		return status
	}
	if status := finishStatus(swap, w); status != "" {
		return status
	}
	if status := acceptStatus(swap, w); status != "" {
		return status
	}
	return ""
}

// unspentOutputs is a struct containing slices of spendable outputs
// categorized by the output type when the outputs are appended to it
// calling the Sort() method sorts all the slices by ascending value so the
// smaller value outputs can be spent first
type unspentOutputs struct {
	scpOutputs  sortableOutputs
	spfAOutputs sortableOutputs
	spfBOutputs sortableOutputs
}

func (so *unspentOutputs) Append(uo modules.UnspentOutput) {
	fmt.Printf("DEBUG: adding %v of %v to unspent outputs\n", uo.Value, uo.FundType.String())
	if uo.FundType == types.SpecifierSiacoinOutput {
		so.scpOutputs = append(so.scpOutputs, uo)
		return
	}
	if uo.FundType == types.SpecifierSiafundOutput {
		so.spfAOutputs = append(so.spfAOutputs, uo)
		return
	}
	if uo.FundType == types.SpecifierSiafundBOutput {
		so.spfBOutputs = append(so.spfBOutputs, uo)
	}
}

func (so *unspentOutputs) Sort() {
	if len(so.spfBOutputs) > 1 {
		sort.Sort(so.spfBOutputs)
	}
	if len(so.spfAOutputs) > 1 {
		sort.Sort(so.spfAOutputs)
	}
	if len(so.scpOutputs) > 1 {
		sort.Sort(so.scpOutputs)
	}
}

type sortableOutputs []modules.UnspentOutput

// Len returns the number of elements in the unspentOutputs struct.
func (so sortableOutputs) Len() int {
	return len(so)
}

// Less returns whether element 'i' is less than element 'j'. The currency
// value of each output is used for comparison.
func (so sortableOutputs) Less(i, j int) bool {
	return so[i].Value.Cmp(so[j].Value) < 0
}

// Swap swaps two elements in the unspentOutputs set.
func (so sortableOutputs) Swap(i, j int) {
	so[i], so[j] = so[j], so[i]
}

// SPFAOutputs returns a slice of unspent SPF-A Outputs
func (so *unspentOutputs) SPFAOutputs() []modules.UnspentOutput {
	return so.spfAOutputs
}

// SPFBOutputs returns a slice of unspent SPF-B Outputs
func (so *unspentOutputs) SPFBOutputs() []modules.UnspentOutput {
	return so.spfBOutputs
}

// SCPOutputs returns a slice of unspent SCP Outputs
func (so *unspentOutputs) SCPOutputs() []modules.UnspentOutput {
	return so.scpOutputs
}

// Outputs returns the outputs corresponding to the given specifier
func (so *unspentOutputs) Outputs(outputType types.Specifier) []modules.UnspentOutput {
	switch outputType {
	case types.SpecifierSiafundOutput:
		return so.spfAOutputs
	case types.SpecifierSiafundBOutput:
		return so.spfBOutputs
	}
	return so.scpOutputs
}
