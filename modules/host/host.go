// Package host is an implementation of the host module, and is responsible for
// participating in the storage ecosystem, turning available disk space an
// internet bandwidth into profit for the user.
package host

// TODO: what happens if the renter submits the revision early, before the
// final revision. Will the host mark the contract as complete?

// TODO: Host and renter are reporting errors where the renter is not adding
// enough fees to the file contract.

// TODO: Test the safety of the builder, it should be okay to have multiple
// builders open for up to 600 seconds, which means multiple blocks could be
// received in that time period. Should also check what happens if a parent
// gets confirmed on the blockchain before the builder is finished.

// TODO: Double check that any network connection has a finite deadline -
// handling action items properly requires that the locks held on the
// obligations eventually be released. There's also some more advanced
// implementation that needs to happen with the storage obligation locks to
// make sure that someone who wants a lock is able to get it eventually.

// TODO: Add contract compensation from form contract to the storage obligation
// financial metrics, and to the host's tracking.

// TODO: merge the network interfaces stuff, don't forget to include the
// 'announced' variable as one of the outputs.

// TODO: 'announced' doesn't tell you if the announcement made it to the
// blockchain.

// TODO: Need to make sure that the revision exchange for the renter and the
// host is being handled correctly. For the host, it's not so difficult. The
// host need only send the most recent revision every time. But, the host
// should not sign a revision unless the renter has explicitly signed such that
// the 'WholeTransaction' fields cover only the revision and that the
// signatures for the revision don't depend on anything else. The renter needs
// to verify the same when checking on a file contract revision from the host.
// If the host has submitted a file contract revision where the signatures have
// signed the whole file contract, there is an issue.

// TODO: there is a mistake in the file contract revision rpc, the host, if it
// does not have the right file contract id, should be returning an error there
// to the renter (and not just to it's calling function without informing the
// renter what's up).

// TODO: Need to make sure that the correct height is being used when adding
// sectors to the storage manager - in some places right now WindowStart is
// being used but really it's WindowEnd that should be in use.

// TODO: The host needs some way to blacklist file contracts that are being
// abusive by repeatedly getting free download batches.

// TODO: clean up all of the magic numbers in the host.

// TODO: revamp the finances for the storage obligations.

// TODO: host_test.go has commented out tests.

// TODO: network_test.go has commented out tests.

// TODO: persist_test.go has commented out tests.

// TODO: update_test.go has commented out tests.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/NebulousLabs/encoding"
	connmonitor "gitlab.com/NebulousLabs/monitor"
	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/host/api"
	"gitlab.com/scpcorp/ScPrime/modules/host/contractmanager"
	"gitlab.com/scpcorp/ScPrime/modules/host/tokenstorage"
	"gitlab.com/scpcorp/ScPrime/persist"
	siasync "gitlab.com/scpcorp/ScPrime/sync"
	"gitlab.com/scpcorp/ScPrime/types"
)

const (
	// Directory name for token storage.
	tokenStorDir = "token_storage"
	// Names of the various persistent files in the host.
	dbFilename   = modules.HostDir + ".db"
	logFile      = modules.HostDir + ".log"
	settingsFile = modules.HostDir + ".json"
)

var (
	// dbMetadata is a header that gets put into the database to identify a
	// version and indicate that the database holds host information.
	dbMetadata = persist.Metadata{
		Header:  "Sia Host DB",
		Version: "0.5.2",
	}

	// Nil dependency errors.
	errNilCS      = errors.New("host cannot use a nil state")
	errNilTpool   = errors.New("host cannot use a nil transaction pool")
	errNilWallet  = errors.New("host cannot use a nil wallet")
	errNilGateway = errors.New("host cannot use nil gateway")

	storageObligationAuditInteval = types.BlocksPerDay // Audit contracts once a day
)

// A Host contains all the fields necessary for storing files for clients and
// performing the storage proofs on the received files.
type Host struct {
	// RPC Metrics - atomic variables need to be placed at the top to preserve
	// compatibility with 32bit systems. These values are not persistent.
	atomicDownloadCalls     uint64
	atomicErroredCalls      uint64
	atomicFormContractCalls uint64
	atomicRenewCalls        uint64
	atomicReviseCalls       uint64
	atomicSettingsCalls     uint64
	atomicUnrecognizedCalls uint64

	// Error management. There are a few different types of errors returned by
	// the host. These errors intentionally not persistent, so that the logging
	// limits of each error type will be reset each time the host is reset.
	// These values are not persistent.
	atomicCommunicationErrors uint64
	atomicConnectionErrors    uint64
	atomicConsensusErrors     uint64
	atomicInternalErrors      uint64
	atomicNormalErrors        uint64

	// Dependencies.
	cs            modules.ConsensusSet
	g             modules.Gateway
	tpool         modules.TransactionPool
	wallet        modules.Wallet
	staticAlerter *modules.GenericAlerter
	dependencies  modules.Dependencies
	// Should be called under mu.RLock and checked for not being nil.
	modules.StorageManager

	// Host ACID fields - these fields need to be updated in serial, ACID
	// transactions.
	announced    bool
	blockHeight  types.BlockHeight
	publicKey    types.SiaPublicKey
	secretKey    crypto.SecretKey
	recentChange modules.ConsensusChangeID
	unlockHash   types.UnlockHash // A wallet address that can receive coins.

	// Host transient fields - these fields are either determined at startup or
	// otherwise are not critical to always be correct.
	autoAddress          modules.NetAddress // Determined using automatic tooling in network.go
	financialMetrics     modules.HostFinancialMetrics
	settings             modules.HostInternalSettings
	revisionNumber       uint64
	workingStatus        modules.HostWorkingStatus
	connectabilityStatus modules.HostConnectabilityStatus
	// scheduledAuditBlockheight is the blockheight when the next AuditStorageObligations()
	// should be started
	scheduledAuditBlockheight uint64

	// A map of storage obligations that are currently being modified. Locks on
	// storage obligations can be long-running, and each storage obligation can
	// be locked separately.
	lockedStorageObligations map[types.FileContractID]*lockedObligation

	// Storage of tokens for prepaid downloads.
	// Should be called under mu.RLock and checked for not being nil.
	tokenStor *tokenstorage.TokenStorage

	// Misc state.
	db            *persist.BoltDatabase
	listener      net.Listener
	log           *persist.Logger
	mu            sync.RWMutex
	staticMonitor *connmonitor.Monitor
	persistDir    string
	port          string
	apiPort       string
	tg            siasync.ThreadGroup

	//atomicReadyToServe is indicator of host initialization comleteness
	// indicates if api calls are going to be served correctly
	atomicReadyToServe atomic.Bool
}

// lockedObligation is a helper type that locks a TryMutex and a counter to
// indicate how many times the locked obligation has been fetched from the
// lockedStorageObligations map already.
type lockedObligation struct {
	mu siasync.TryMutex
	n  uint
}

// checkUnlockHash will check that the host has an unlock hash. If the host
// does not have an unlock hash, an attempt will be made to get an unlock hash
// from the wallet. That may fail due to the wallet being locked, in which case
// an error is returned.
func (h *Host) checkUnlockHash() error {
	addrs, err := h.wallet.AllAddresses()
	if err != nil {
		return err
	}
	hasAddr := false
	for _, addr := range addrs {
		if h.unlockHash == addr {
			hasAddr = true
			break
		}
	}
	if !hasAddr || h.unlockHash == (types.UnlockHash{}) {
		uc, err := h.wallet.NextAddress()
		if err != nil {
			return err
		}

		// Set the unlock hash and save the host. Saving is important, because
		// the host will be using this unlock hash to establish identity, and
		// losing it will mean silently losing part of the host identity.
		h.unlockHash = uc.UnlockHash()
		err = h.saveSync()
		if err != nil {
			return err
		}
	}
	return nil
}

// managedInternalSettings returns the settings of a host.
func (h *Host) managedInternalSettings() modules.HostInternalSettings {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.settings
}

// newHost returns an initialized Host, taking a set of dependencies as input.
// By making the dependencies an argument of the 'new' call, the host can be
// mocked such that the dependencies can return unexpected errors or unique
// behaviors during testing, enabling easier testing of the failure modes of
// the Host.
func newHost(dependencies modules.Dependencies, smDeps modules.Dependencies, cs modules.ConsensusSet, g modules.Gateway,
	tpool modules.TransactionPool, wallet modules.Wallet, listenerAddress, persistDir string,
	hostAPIListener net.Listener, checkTokenExpirationFrequency time.Duration, onlyFirstDir bool) (*Host, error) {
	// Check that all the dependencies were provided.
	if cs == nil {
		return nil, errNilCS
	}
	if g == nil {
		return nil, errNilGateway
	}
	if tpool == nil {
		return nil, errNilTpool
	}
	if wallet == nil {
		return nil, errNilWallet
	}

	var hostAPIPort string
	var err error
	if hostAPIListener == nil {
		// Get free port.
		hostAPIListener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, fmt.Errorf("make listener for host api2 port: %w", err)
		}
	}
	hostAPIPort = ":" + strconv.Itoa(hostAPIListener.Addr().(*net.TCPAddr).Port)

	// Create the host object.
	h := &Host{
		cs:                       cs,
		g:                        g,
		tpool:                    tpool,
		wallet:                   wallet,
		staticAlerter:            modules.NewAlerter("host"),
		dependencies:             dependencies,
		lockedStorageObligations: make(map[types.FileContractID]*lockedObligation),
		persistDir:               persistDir,
		apiPort:                  hostAPIPort,
	}
	h.atomicReadyToServe.Store(false)

	// Call stop in the event of a partial startup.
	defer func() {
		if err != nil {
			err = composeErrors(h.tg.Stop(), err)
		}
	}()

	// Create the perist directory if it does not yet exist.
	err = dependencies.MkdirAll(h.persistDir, 0700)
	if err != nil {
		return nil, fmt.Errorf("could not create directory %v: %w", h.persistDir, err)
	}

	// Initialize the logger, and set up the stop call that will close the
	// logger.
	h.log, err = dependencies.NewLogger(filepath.Join(h.persistDir, logFile))
	if err != nil {
		return nil, fmt.Errorf("Error creating logger: %w", err)
	}

	h.tg.AfterStop(func() {
		err = h.log.Close()
		if err != nil {
			// State of the logger is uncertain, a Println will have to
			// suffice.
			fmt.Println("Error when closing the logger:", err)
		}
	})

	tokenStorageDir := filepath.Join(h.persistDir, tokenStorDir)
	if _, err = os.Stat(tokenStorageDir); os.IsNotExist(err) {
		// Create the token storage directory if it does not yet exist.
		if err = os.Mkdir(tokenStorageDir, 0755); err != nil {
			return nil, fmt.Errorf("error creating token storage directory: %w", err)
		}
	}
	h.log.Debugf("Token storage directory (%v) ready", tokenStorageDir)
	// Add the storage manager to the host, and set up the stop call that will
	// close the storage manager.
	stManager, err := contractmanager.NewCustomContractManager(smDeps, filepath.Join(persistDir, "contractmanager"), onlyFirstDir)
	if err != nil {
		h.log.Println("Could not open the storage manager:", err)
		return nil, fmt.Errorf("error creating contract manager: %w", err)
	}
	h.StorageManager = stManager
	h.log.Debugf("Contract manager ready")
	h.tg.AfterStop(func() {
		err = h.StorageManager.Close()
		if err != nil {
			h.log.Println("Could not close storage manager:", err)
			err = fmt.Errorf("error closing the storage manager: %w", err)
		}
	})

	// Initialize token storage.
	// TODO: the current version of tokens storage does not support reverting blocks.
	// If contracts related to `TopUpToken` RPC are reverted, all the tokens resources remain in the storage.
	h.tokenStor, err = tokenstorage.NewTokenStorage(stManager, tokenStorageDir)
	if err != nil {
		return nil, fmt.Errorf("error initializing token storage: %w", err)
	}
	h.log.Debugf("TokenStorage manager ready")
	updateTokenSectorsChan := make(chan bool)
	h.tg.AfterStop(func() {
		updateTokenSectorsChan <- true
		err = h.tokenStor.Close(context.Background())
		if err != nil {
			h.log.Errorf("Error when closing token storage: %v", err)
		}
	})
	// Remove sectors from token when token storage resource ends.
	go h.tokenStor.CheckExpiration(checkTokenExpirationFrequency, updateTokenSectorsChan)

	// Load the prior persistence structures, and configure the host to save
	// before shutting down.
	err = h.load()
	if err != nil {
		return nil, fmt.Errorf("error loading persistence: %w", err)
	}
	h.log.Debugf("Host persistence loaded")
	h.tg.AfterStop(func() {
		err = h.saveSync()
		if err != nil {
			h.log.Errorf("Error saving host upon shutdown: %v", err)
		}
	})

	atomic.StoreUint64(&h.scheduledAuditBlockheight, uint64(h.blockHeight))
	h.log.Debugf("h.scheduledAuditBlockheight set to %v", h.blockHeight)
	// Subscribe to the consensus set.
	err = h.initConsensusSubscription()
	if err != nil {
		return nil, fmt.Errorf("error subscribing to consensus: %w", err)
	}
	h.log.Debugln("Consensus subscription initialized")

	// Create bandwidth monitor
	h.staticMonitor = connmonitor.NewMonitor()

	// Initialize the networking. We need to hold the lock while doing so since
	// the previous load subscribed the host to the consensus set.
	h.mu.Lock()
	err = h.initNetworking(listenerAddress)
	h.mu.Unlock()
	if err != nil {
		h.log.Errorf("Could not initialize host networking: %v", err)
		return nil, err
	}

	//	Initialize and run host API
	hostApi := api.NewAPI(h.tokenStor, h.secretKey, h)
	err = hostApi.Start(hostAPIListener)
	if err != nil {
		err = fmt.Errorf("error starting host api: %w", err)
		h.log.Errorln(err)
		return nil, err
	}
	h.tg.AfterStop(func() {
		err = hostApi.Close()
		if err != nil {
			h.log.Errorln("Could not close host API:", err)
			err = fmt.Errorf("error closing host API: %w", err)
		}
	})
	//remove obsoleted ephemeral accounts and fingerprintsbucket files if there are any
	h.log.Debugf("Removing fingerprintsbucket files from %v", h.persistDir)
	err = removeObsoletedFiles(h.persistDir)
	if err != nil {
		h.log.Debugf("remove fingerprintbuckets files failed: %v", err)
	}
	h.atomicReadyToServe.Store(true)
	return h, nil
}

// New returns an initialized Host.
func New(cs modules.ConsensusSet, g modules.Gateway, tpool modules.TransactionPool, wallet modules.Wallet, address, persistDir string, hostAPIListener net.Listener, checkTokenExpirationFrequency time.Duration) (*Host, error) {
	const onlyFirstDir = false
	return newHost(modules.ProdDependencies, new(modules.ProductionDependencies), cs, g, tpool, wallet, address, persistDir, hostAPIListener, checkTokenExpirationFrequency, onlyFirstDir)
}

// NewCustomHost returns an initialized Host using the provided dependencies.
func NewCustomHost(deps modules.Dependencies, cs modules.ConsensusSet, g modules.Gateway, tpool modules.TransactionPool, wallet modules.Wallet, address, persistDir string, hostAPIListener net.Listener, checkTokenExpirationFrequency time.Duration) (*Host, error) {
	const onlyFirstDir = false
	return newHost(deps, new(modules.ProductionDependencies), cs, g, tpool, wallet, address, persistDir, hostAPIListener, checkTokenExpirationFrequency, onlyFirstDir)
}

// NewCustomTestHost allows passing in both host dependencies and storage
// manager dependencies. Used solely for testing purposes, to allow dependency
// injection into the host's submodules.
func NewCustomTestHost(deps modules.Dependencies, smDeps modules.Dependencies, cs modules.ConsensusSet, g modules.Gateway, tpool modules.TransactionPool, wallet modules.Wallet, address, persistDir string, hostAPIListener net.Listener, checkTokenExpirationFrequency time.Duration, onlyFirstDir bool) (*Host, error) {
	return newHost(deps, smDeps, cs, g, tpool, wallet, address, persistDir, hostAPIListener, checkTokenExpirationFrequency, onlyFirstDir)
}

// Close shuts down the host.
func (h *Host) Close() error {
	return h.tg.Stop()
}

// Announcement returns host announcement.
// For use as ghost.
func (h *Host) Announcement() []byte {
	ann := encoding.Marshal(modules.HostAnnouncement{
		Specifier:  modules.PrefixHostAnnouncement,
		NetAddress: modules.NetAddress(h.listener.Addr().String()),
		PublicKey:  h.PublicKey(),
	})
	sig := crypto.SignHash(crypto.HashBytes(ann), h.secretKey)
	return append(ann, sig[:]...)
}

// ExternalSettings returns the hosts external settings. These values cannot be
// set by the user (host is configured through InternalSettings), and are the
// values that get displayed to other hosts on the network.
func (h *Host) ExternalSettings() modules.HostExternalSettings {
	err := h.tg.Add()
	if err != nil {
		build.Critical("Call to ExternalSettings after close")
	}
	defer h.tg.Done()
	return h.ManagedExternalSettings()
}

// BandwidthCounters returns the Hosts's upload and download bandwidth
func (h *Host) BandwidthCounters() (uint64, uint64, time.Time, error) {
	if err := h.tg.Add(); err != nil {
		return 0, 0, time.Time{}, err
	}
	defer h.tg.Done()

	// Get the bandwidth usage for RHP1 & RHP2 connections.
	readBytes, writeBytes := h.staticMonitor.Counts()

	startTime := h.staticMonitor.StartTime()
	return writeBytes, readBytes, startTime, nil
}

// WorkingStatus returns the working state of the host, where working is
// defined as having received more than workingStatusThreshold settings calls
// over the period of workingStatusFrequency.
func (h *Host) WorkingStatus() modules.HostWorkingStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.workingStatus
}

// ConnectabilityStatus returns the connectability state of the host, whether
// the host can connect to itself on its configured netaddress.
func (h *Host) ConnectabilityStatus() modules.HostConnectabilityStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.connectabilityStatus
}

// FinancialMetrics returns information about the financial commitments,
// rewards, and activities of the host.
func (h *Host) FinancialMetrics() modules.HostFinancialMetrics {
	err := h.tg.Add()
	if err != nil {
		build.Critical("Call to FinancialMetrics after close")
	}
	defer h.tg.Done()
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.financialMetrics
}

// PublicKey returns the public key of the host that is used to facilitate
// relationships between the host and renter.
func (h *Host) PublicKey() types.SiaPublicKey {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.publicKey
}

// SetInternalSettings updates the host's internal HostInternalSettings object.
func (h *Host) SetInternalSettings(settings modules.HostInternalSettings) error {
	err := h.tg.Add()
	if err != nil {
		return err
	}
	defer h.tg.Done()

	h.mu.Lock()
	// By updating the internal settings the user might influence the host's
	// price table, we defer a call to update the price table to ensure it
	// reflects the updated settings.
	//defer h.managedUpdatePriceTable()
	defer h.mu.Unlock()

	// The host should not be accepting file contracts if it does not have an
	// unlock hash.
	if settings.AcceptingContracts {
		err := h.checkUnlockHash()
		if err != nil {
			return errors.New("internal settings not updated, no unlock hash: " + err.Error())
		}
	}

	if settings.NetAddress != "" {
		err := settings.NetAddress.IsValid()
		if err != nil {
			return errors.New("internal settings not updated, invalid NetAddress: " + err.Error())
		}
	}

	// Check if the net address for the host has changed. If it has, and it's
	// not equal to the auto address, then the host is going to need to make
	// another blockchain announcement.
	if h.settings.NetAddress != settings.NetAddress && settings.NetAddress != h.autoAddress {
		h.announced = false
	}

	h.settings = settings
	h.revisionNumber++

	// The locked storage collateral was altered, we potentially want to
	// unregister the insufficient collateral budget alert
	h.tryUnregisterInsufficientCollateralBudgetAlert()

	err = h.saveSync()
	if err != nil {
		return errors.New("internal settings updated, but failed saving to disk: " + err.Error())
	}
	return nil
}

// InternalSettings returns the settings of a host.
func (h *Host) InternalSettings() modules.HostInternalSettings {
	err := h.tg.Add()
	if err != nil {
		return modules.HostInternalSettings{}
	}
	defer h.tg.Done()
	return h.managedInternalSettings()
}

// BlockHeight returns the host's current blockheight.
func (h *Host) BlockHeight() types.BlockHeight {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.blockHeight
}

// ManagedExternalSettings returns the host's external settings. These values
// cannot be set by the user (host is configured through InternalSettings), and
// are the values that get displayed to other hosts on the network.
func (h *Host) ManagedExternalSettings() modules.HostExternalSettings {
	_, maxFee := h.tpool.FeeEstimation()
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.externalSettings(maxFee)
}

// ReadyToServe tells if the host module is ready to serve API calls.
func (h *Host) ReadyToServe() bool {
	return h.atomicReadyToServe.Load()
}
