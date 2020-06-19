package contractor

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/siamux"
	"gitlab.com/NebulousLabs/threadgroup"

	"gitlab.com/NebulousLabs/Sia/build"
	"gitlab.com/NebulousLabs/Sia/crypto"
	"gitlab.com/NebulousLabs/Sia/modules"
	"gitlab.com/NebulousLabs/Sia/modules/renter/proto"
	"gitlab.com/NebulousLabs/Sia/persist"
	siasync "gitlab.com/NebulousLabs/Sia/sync"
	"gitlab.com/NebulousLabs/Sia/types"
)

var (
	errNilCS     = errors.New("cannot create contractor with nil consensus set")
	errNilHDB    = errors.New("cannot create contractor with nil HostDB")
	errNilTpool  = errors.New("cannot create contractor with nil transaction pool")
	errNilWallet = errors.New("cannot create contractor with nil wallet")

	errHostNotFound         = errors.New("host not found")
	errContractNotFound     = errors.New("contract not found")
	errRefundAccountInvalid = errors.New("invalid refund account")

	// COMPATv1.0.4-lts
	// metricsContractID identifies a special contract that contains aggregate
	// financial metrics from older contractors
	metricsContractID = types.FileContractID{'m', 'e', 't', 'r', 'i', 'c', 's'}
)

// A Contractor negotiates, revises, renews, and provides access to file
// contracts.
type Contractor struct {
	// dependencies
	cs            modules.ConsensusSet
	hdb           modules.HostDB
	log           *persist.Logger
	mu            sync.RWMutex
	persistDir    string
	staticAlerter *modules.GenericAlerter
	staticDeps    modules.Dependencies
	tg            siasync.ThreadGroup
	tpool         modules.TransactionPool
	wallet        modules.Wallet

	// Only one thread should be performing contract maintenance at a time.
	interruptMaintenance chan struct{}
	maintenanceLock      siasync.TryMutex

	// Only one thread should be scanning the blockchain for recoverable
	// contracts at a time.
	atomicScanInProgress     uint32
	atomicRecoveryScanHeight int64

	allowance     modules.Allowance
	blockHeight   types.BlockHeight
	synced        chan struct{}
	currentPeriod types.BlockHeight
	lastChange    modules.ConsensusChangeID

	// recentRecoveryChange is the first ConsensusChange that was missed while
	// trying to find recoverable contracts. This is where we need to start
	// rescanning the blockchain for recoverable contracts the next time the wallet
	// is unlocked.
	recentRecoveryChange modules.ConsensusChangeID

	downloaders     map[types.FileContractID]*hostDownloader
	editors         map[types.FileContractID]*hostEditor
	sessions        map[types.FileContractID]*hostSession
	numFailedRenews map[types.FileContractID]types.BlockHeight
	renewing        map[types.FileContractID]bool // prevent revising during renewal

	// pubKeysToContractID is a map of host pubkeys to the latest contract ID
	// that is formed with the host. The contract also has to have an end height
	// in the future
	pubKeysToContractID map[string]types.FileContractID

	// renewedFrom links the new contract's ID to the old contract's ID
	// renewedTo links the old contract's ID to the new contract's ID
	// doubleSpentContracts keep track of all contracts that were double spent by
	// either the renter or host.
	staticContracts      *proto.ContractSet
	oldContracts         map[types.FileContractID]modules.RenterContract
	doubleSpentContracts map[types.FileContractID]types.BlockHeight
	recoverableContracts map[types.FileContractID]modules.RecoverableContract
	renewedFrom          map[types.FileContractID]types.FileContractID
	renewedTo            map[types.FileContractID]types.FileContractID

	staticChurnLimiter *churnLimiter
	staticWatchdog     *watchdog
}

// Allowance returns the current allowance.
func (c *Contractor) Allowance() modules.Allowance {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.allowance
}

// InitRecoveryScan starts scanning the whole blockchain for recoverable
// contracts within a separate thread.
func (c *Contractor) InitRecoveryScan() (err error) {
	if err := c.tg.Add(); err != nil {
		return err
	}
	defer c.tg.Done()
	return c.callInitRecoveryScan(modules.ConsensusChangeBeginning)
}

// PeriodSpending returns the amount spent on contracts during the current
// billing period.
func (c *Contractor) PeriodSpending() (modules.ContractorSpending, error) {
	allContracts := c.staticContracts.ViewAll()
	c.mu.RLock()
	defer c.mu.RUnlock()

	var spending modules.ContractorSpending
	for _, contract := range allContracts {
		// Don't count double-spent contracts.
		if _, doubleSpent := c.doubleSpentContracts[contract.ID]; doubleSpent {
			continue
		}

		// Calculate ContractFees
		spending.ContractFees = spending.ContractFees.Add(contract.ContractFee)
		spending.ContractFees = spending.ContractFees.Add(contract.TxnFee)
		spending.ContractFees = spending.ContractFees.Add(contract.SiafundFee)
		// Calculate TotalAllocated
		spending.TotalAllocated = spending.TotalAllocated.Add(contract.TotalCost)
		spending.ContractSpendingDeprecated = spending.TotalAllocated
		// Calculate Spending
		spending.DownloadSpending = spending.DownloadSpending.Add(contract.DownloadSpending)
		spending.UploadSpending = spending.UploadSpending.Add(contract.UploadSpending)
		spending.StorageSpending = spending.StorageSpending.Add(contract.StorageSpending)
	}

	// Calculate needed spending to be reported from old contracts
	for _, contract := range c.oldContracts {
		// Don't count double-spent contracts.
		if _, doubleSpent := c.doubleSpentContracts[contract.ID]; doubleSpent {
			continue
		}

		host, exist, err := c.hdb.Host(contract.HostPublicKey)
		if contract.StartHeight >= c.currentPeriod {
			// Calculate spending from contracts that were renewed during the current period
			// Calculate ContractFees
			spending.ContractFees = spending.ContractFees.Add(contract.ContractFee)
			spending.ContractFees = spending.ContractFees.Add(contract.TxnFee)
			spending.ContractFees = spending.ContractFees.Add(contract.SiafundFee)
			// Calculate TotalAllocated
			spending.TotalAllocated = spending.TotalAllocated.Add(contract.TotalCost)
			// Calculate Spending
			spending.DownloadSpending = spending.DownloadSpending.Add(contract.DownloadSpending)
			spending.UploadSpending = spending.UploadSpending.Add(contract.UploadSpending)
			spending.StorageSpending = spending.StorageSpending.Add(contract.StorageSpending)
		} else if err != nil && exist && contract.EndHeight+host.WindowSize+types.MaturityDelay > c.blockHeight {
			// Calculate funds that are being withheld in contracts
			spending.WithheldFunds = spending.WithheldFunds.Add(contract.RenterFunds)
			// Record the largest window size for worst case when reporting the spending
			if contract.EndHeight+host.WindowSize+types.MaturityDelay >= spending.ReleaseBlock {
				spending.ReleaseBlock = contract.EndHeight + host.WindowSize + types.MaturityDelay
			}
			// Calculate Previous spending
			spending.PreviousSpending = spending.PreviousSpending.Add(contract.ContractFee).Add(contract.TxnFee).
				Add(contract.SiafundFee).Add(contract.DownloadSpending).Add(contract.UploadSpending).Add(contract.StorageSpending)
		} else {
			// Calculate Previous spending
			spending.PreviousSpending = spending.PreviousSpending.Add(contract.ContractFee).Add(contract.TxnFee).
				Add(contract.SiafundFee).Add(contract.DownloadSpending).Add(contract.UploadSpending).Add(contract.StorageSpending)
		}
	}

	// Calculate amount of spent money to get unspent money.
	allSpending := spending.ContractFees
	allSpending = allSpending.Add(spending.DownloadSpending)
	allSpending = allSpending.Add(spending.UploadSpending)
	allSpending = allSpending.Add(spending.StorageSpending)
	if c.allowance.Funds.Cmp(allSpending) >= 0 {
		spending.Unspent = c.allowance.Funds.Sub(allSpending)
	}

	return spending, nil
}

// CurrentPeriod returns the height at which the current allowance period
// began.
func (c *Contractor) CurrentPeriod() types.BlockHeight {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentPeriod
}

// ProvidePayment fulfills the PaymentProvider interface. It uses the given
// stream and necessary payment details to perform payment for an RPC call.
func (c *Contractor) ProvidePayment(stream siamux.Stream, host types.SiaPublicKey, rpc types.Specifier, amount types.Currency, refundAccount modules.AccountID, blockHeight types.BlockHeight) error {
	// verify we do not specify a refund account on the fund account RPC
	if rpc == modules.RPCFundAccount && !refundAccount.IsZeroAccount() {
		return errRefundAccountInvalid
	}

	// find a contract for the given host
	contract, exists := c.ContractByPublicKey(host)
	if !exists {
		return errContractNotFound
	}

	// acquire a safe contract
	sc, exists := c.staticContracts.Acquire(contract.ID)
	if !exists {
		return errContractNotFound
	}
	defer c.staticContracts.Return(sc)

	// create a new revision
	current := sc.LastRevision()
	rev, err := current.EAFundRevision(amount)
	if err != nil {
		return errors.AddContext(err, "Failed to create a payment revision")
	}

	// create transaction containing the revision
	signedTxn := rev.ToTransaction()
	sig := sc.Sign(signedTxn.SigHash(0, blockHeight))
	signedTxn.TransactionSignatures[0].Signature = sig[:]

	// record the payment intent
	walTxn, err := sc.RecordPaymentIntent(rev, amount, rpc)
	if err != nil {
		return errors.AddContext(err, "Failed to record payment intent")
	}

	// prepare a buffer so we can optimize our writes
	buffer := bytes.NewBuffer(nil)

	// send PaymentRequest
	err = modules.RPCWrite(buffer, modules.PaymentRequest{Type: modules.PayByContract})
	if err != nil {
		return errors.AddContext(err, "unable to write payment request to host")
	}

	// send PayByContractRequest
	err = modules.RPCWrite(buffer, newPayByContractRequest(rev, sig, refundAccount))
	if err != nil {
		return errors.AddContext(err, "unable to write the pay by contract request")
	}

	// write contents of the buffer to the stream
	_, err = stream.Write(buffer.Bytes())
	if err != nil {
		return errors.AddContext(err, "could not write the buffer contents")
	}

	// receive PayByContractResponse
	var payByResponse modules.PayByContractResponse
	if err := modules.RPCRead(stream, &payByResponse); err != nil {
		return errors.AddContext(err, "unable to read the pay by contract response")
	}

	// TODO: Check for revision mismatch and recover by applying the contract
	// unapplied transactions and trying again.

	// verify the host's signature
	hash := crypto.HashAll(rev)
	hpk := sc.Metadata().HostPublicKey
	err = crypto.VerifyHash(hash, hpk.ToPublicKey(), payByResponse.Signature)
	if err != nil {
		return errors.New("could not verify host's signature")
	}

	// commit payment intent
	err = sc.CommitPaymentIntent(walTxn, signedTxn, amount, rpc)
	if err != nil {
		return errors.AddContext(err, "Failed to commit unknown spending intent")
	}

	return nil
}

// RateLimits sets the bandwidth limits for connections created by the
// contractSet.
func (c *Contractor) RateLimits() (readBPW int64, writeBPS int64, packetSize uint64) {
	return c.staticContracts.RateLimits()
}

// RecoveryScanStatus returns a bool indicating if a scan for recoverable
// contracts is in progress and if it is, the current progress of the scan.
func (c *Contractor) RecoveryScanStatus() (bool, types.BlockHeight) {
	bh := types.BlockHeight(atomic.LoadInt64(&c.atomicRecoveryScanHeight))
	sip := atomic.LoadUint32(&c.atomicScanInProgress)
	return sip == 1, bh
}

// RefreshedContract returns a bool indicating if the contract was a refreshed
// contract. A refreshed contract refers to a contract that ran out of funds
// prior to the end height and so was renewed with the host in the same period.
// Both the old and the new contract have the same end height
func (c *Contractor) RefreshedContract(fcid types.FileContractID) bool {
	// Add thread and acquire lock
	if err := c.tg.Add(); err != nil {
		return false
	}
	defer c.tg.Done()
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check if contract ID is found in the renewedTo map indicating that the
	// contract was renewed
	newFCID, renewed := c.renewedTo[fcid]
	if !renewed {
		return false
	}

	// Grab the contract to check its end height
	contract, ok := c.oldContracts[fcid]
	if !ok {
		c.log.Debugln("Contract not found in oldContracts, despite there being a renewal to the contract")
		return false
	}

	// Grab the contract it was renewed to to check its end height
	newContract, ok := c.staticContracts.View(newFCID)
	if !ok {
		newContract, ok = c.oldContracts[newFCID]
		if !ok {
			c.log.Debugln("Contract was not found in the database, despite their being another contract that claims to have renewed to it.")
			return false
		}
	}

	// The contract was refreshed if the endheights are the same
	return newContract.EndHeight == contract.EndHeight
}

// SetRateLimits sets the bandwidth limits for connections created by the
// contractSet.
func (c *Contractor) SetRateLimits(readBPS int64, writeBPS int64, packetSize uint64) {
	c.staticContracts.SetRateLimits(readBPS, writeBPS, packetSize)
}

// Synced returns a channel that is closed when the contractor is synced with
// the peer-to-peer network.
func (c *Contractor) Synced() <-chan struct{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.synced
}

// Close closes the Contractor.
func (c *Contractor) Close() error {
	return c.tg.Stop()
}

// New returns a new Contractor.
func New(cs modules.ConsensusSet, wallet modules.Wallet, tpool modules.TransactionPool, hdb modules.HostDB, persistDir string) (*Contractor, <-chan error) {
	errChan := make(chan error, 1)
	defer close(errChan)
	// Check for nil inputs.
	if cs == nil {
		errChan <- errNilCS
		return nil, errChan
	}
	if wallet == nil {
		errChan <- errNilWallet
		return nil, errChan
	}
	if tpool == nil {
		errChan <- errNilTpool
		return nil, errChan
	}
	if hdb == nil {
		errChan <- errNilHDB
		return nil, errChan
	}

	// Create the persist directory if it does not yet exist.
	if err := os.MkdirAll(persistDir, 0700); err != nil {
		errChan <- err
		return nil, errChan
	}

	// Convert the old persist file(s), if necessary. This must occur before
	// loading the contract set.
	if err := convertPersist(persistDir); err != nil {
		errChan <- err
		return nil, errChan
	}

	// Create the contract set.
	contractSet, err := proto.NewContractSet(filepath.Join(persistDir, "contracts"), modules.ProdDependencies)
	if err != nil {
		errChan <- err
		return nil, errChan
	}
	// Create the logger.
	logger, err := persist.NewFileLogger(filepath.Join(persistDir, "contractor.log"))
	if err != nil {
		errChan <- err
		return nil, errChan
	}

	// Create Contractor using production dependencies.
	return NewCustomContractor(cs, wallet, tpool, hdb, persistDir, contractSet, logger, modules.ProdDependencies)
}

// contractorBlockingStartup handles the blocking portion of NewCustomContractor.
func contractorBlockingStartup(cs modules.ConsensusSet, w modules.Wallet, tp modules.TransactionPool, hdb modules.HostDB, persistDir string, contractSet *proto.ContractSet, l *persist.Logger, deps modules.Dependencies) (*Contractor, error) {
	// Create the Contractor object.
	c := &Contractor{
		staticAlerter: modules.NewAlerter("contractor"),
		cs:            cs,
		staticDeps:    deps,
		hdb:           hdb,
		log:           l,
		persistDir:    persistDir,
		tpool:         tp,
		wallet:        w,

		interruptMaintenance: make(chan struct{}),
		synced:               make(chan struct{}),

		staticContracts:      contractSet,
		downloaders:          make(map[types.FileContractID]*hostDownloader),
		editors:              make(map[types.FileContractID]*hostEditor),
		sessions:             make(map[types.FileContractID]*hostSession),
		oldContracts:         make(map[types.FileContractID]modules.RenterContract),
		doubleSpentContracts: make(map[types.FileContractID]types.BlockHeight),
		recoverableContracts: make(map[types.FileContractID]modules.RecoverableContract),
		renewing:             make(map[types.FileContractID]bool),
		renewedFrom:          make(map[types.FileContractID]types.FileContractID),
		renewedTo:            make(map[types.FileContractID]types.FileContractID),
	}
	c.staticChurnLimiter = newChurnLimiter(c)
	c.staticWatchdog = newWatchdog(c)

	// Close the contract set and logger upon shutdown.
	c.tg.AfterStop(func() {
		if err := c.staticContracts.Close(); err != nil {
			c.log.Println("Failed to close contract set:", err)
		}
		if err := c.log.Close(); err != nil {
			fmt.Println("Failed to close the contractor logger:", err)
		}
	})

	// Load the prior persistence structures.
	err := c.load()
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	// Update the pubkeyToContractID map
	c.managedUpdatePubKeyToContractIDMap()

	// Unsubscribe from the consensus set upon shutdown.
	c.tg.OnStop(func() {
		cs.Unsubscribe(c)
	})

	// We may have upgraded persist or resubscribed. Save now so that we don't
	// lose our work.
	c.mu.Lock()
	err = c.save()
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}

	// Update the allowance in the hostdb with the one that was loaded from
	// disk.
	err = c.hdb.SetAllowance(c.allowance)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// contractorAsyncStartup handles the async portion of NewCustomContractor.
func contractorAsyncStartup(c *Contractor, cs modules.ConsensusSet) error {
	if c.staticDeps.Disrupt("BlockAsyncStartup") {
		return nil
	}
	err := cs.ConsensusSetSubscribe(c, c.lastChange, c.tg.StopChan())
	if err == modules.ErrInvalidConsensusChangeID {
		// Reset the contractor consensus variables and try rescanning.
		c.blockHeight = 0
		c.lastChange = modules.ConsensusChangeBeginning
		c.recentRecoveryChange = modules.ConsensusChangeBeginning
		err = cs.ConsensusSetSubscribe(c, c.lastChange, c.tg.StopChan())
	}
	if err != nil && strings.Contains(err.Error(), threadgroup.ErrStopped.Error()) {
		return nil
	}
	if err != nil {
		return err
	}
	return nil
}

// NewCustomContractor creates a Contractor using the provided dependencies.
func NewCustomContractor(cs modules.ConsensusSet, w modules.Wallet, tp modules.TransactionPool, hdb modules.HostDB, persistDir string, contractSet *proto.ContractSet, l *persist.Logger, deps modules.Dependencies) (*Contractor, <-chan error) {
	errChan := make(chan error, 1)

	// Handle blocking startup.
	c, err := contractorBlockingStartup(cs, w, tp, hdb, persistDir, contractSet, l, deps)
	if err != nil {
		errChan <- err
		return nil, errChan
	}

	// non-blocking startup.
	go func() {
		// Subscribe to the consensus set in a separate goroutine.
		defer close(errChan)
		if err := c.tg.Add(); err != nil {
			errChan <- err
			return
		}
		defer c.tg.Done()
		err := contractorAsyncStartup(c, cs)
		if err != nil {
			errChan <- err
		}
	}()
	return c, errChan
}

// callInitRecoveryScan starts scanning the whole blockchain at a certain
// ChangeID for recoverable contracts within a separate thread.
func (c *Contractor) callInitRecoveryScan(scanStart modules.ConsensusChangeID) (err error) {
	// Check if we are already scanning the blockchain.
	if !atomic.CompareAndSwapUint32(&c.atomicScanInProgress, 0, 1) {
		return errors.New("scan for recoverable contracts is already in progress")
	}
	// Reset the progress and status if there was an error.
	defer func() {
		if err != nil {
			atomic.StoreUint32(&c.atomicScanInProgress, 0)
			atomic.StoreInt64(&c.atomicRecoveryScanHeight, 0)
		}
	}()
	// Get the wallet seed.
	s, _, err := c.wallet.PrimarySeed()
	if err != nil {
		return errors.AddContext(err, "failed to get wallet seed")
	}
	// Get the renter seed and wipe it once done.
	rs := proto.DeriveRenterSeed(s)
	// Reset the scan progress before starting the scan.
	atomic.StoreInt64(&c.atomicRecoveryScanHeight, 0)
	// Create the scanner.
	scanner := c.newRecoveryScanner(rs)
	// Start the scan.
	go func() {
		// Add scanning thread to threadgroup.
		if err := c.tg.Add(); err != nil {
			return
		}
		defer c.tg.Done()
		// Scan blockchain.
		if err := scanner.threadedScan(c.cs, scanStart, c.tg.StopChan()); err != nil {
			c.log.Println("Scan failed", err)
		}
		if c.staticDeps.Disrupt("disableRecoveryStatusReset") {
			return
		}
		// Reset the scan related fields.
		if !atomic.CompareAndSwapUint32(&c.atomicScanInProgress, 1, 0) {
			build.Critical("finished recovery scan but scanInProgress was already set to 0")
		}
		atomic.StoreInt64(&c.atomicRecoveryScanHeight, 0)
		// Save the renter.
		c.mu.Lock()
		c.save()
		c.mu.Unlock()
	}()
	return nil
}

// managedSynced returns true if the contractor is synced with the consensusset.
func (c *Contractor) managedSynced() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	select {
	case <-c.synced:
		return true
	default:
	}
	return false
}

// newPayByContractRequest is a helper function that takes a revision,
// signature and refund account and creates a PayByContractRequest object.
func newPayByContractRequest(rev types.FileContractRevision, sig crypto.Signature, refundAccount modules.AccountID) modules.PayByContractRequest {
	req := modules.PayByContractRequest{
		ContractID:           rev.ID(),
		NewRevisionNumber:    rev.NewRevisionNumber,
		NewValidProofValues:  make([]types.Currency, len(rev.NewValidProofOutputs)),
		NewMissedProofValues: make([]types.Currency, len(rev.NewMissedProofOutputs)),
		RefundAccount:        refundAccount,
		Signature:            sig[:],
	}
	for i, o := range rev.NewValidProofOutputs {
		req.NewValidProofValues[i] = o.Value
	}
	for i, o := range rev.NewMissedProofOutputs {
		req.NewMissedProofValues[i] = o.Value
	}
	return req
}
