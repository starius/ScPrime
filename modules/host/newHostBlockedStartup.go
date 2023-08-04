package host

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	connmonitor "gitlab.com/NebulousLabs/monitor"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/host/api"
	"gitlab.com/scpcorp/ScPrime/modules/host/contractmanager"
	"gitlab.com/scpcorp/ScPrime/modules/host/tokenstorage"
	"gitlab.com/scpcorp/ScPrime/types"
)

// newHostBlocked returns a Host
// but delays the loading and starting until the consensus is synced
func newHostBlocked(dependencies modules.Dependencies, smDeps modules.Dependencies, cs modules.ConsensusSet, g modules.Gateway,
	tpool modules.TransactionPool, wallet modules.Wallet, listenerAddress, persistDir string,
	hostAPIListener net.Listener, checkTokenExpirationFrequency time.Duration, onlyFirstDir bool) (*Host, <-chan error) {
	errChan := make(chan error, 1) //errors will be sent there
	//delegate errChan closing to exit of blocked loader
	//defer close(errChan)

	// Check that all the dependencies were provided.
	if cs == nil {
		errChan <- errNilCS
		return nil, errChan
	}
	if g == nil {
		errChan <- errNilGateway
		return nil, errChan
	}
	if tpool == nil {
		errChan <- errNilTpool
		return nil, errChan
	}
	if wallet == nil {
		errChan <- errNilWallet
		return nil, errChan
	}

	var hostAPIPort string
	var err error
	if hostAPIListener == nil {
		// Get free port.
		hostAPIListener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			errChan <- fmt.Errorf("make listener for host api2 port: %w", err)
			return nil, errChan
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
			errChan <- composeErrors(h.tg.Stop(), err)
		}
	}()

	// Create the perist directory if it does not yet exist.
	err = dependencies.MkdirAll(h.persistDir, 0700)
	if err != nil {
		errChan <- fmt.Errorf("could not create directory %v: %w", h.persistDir, err)
		return nil, errChan
	}

	// Initialize the logger, and set up the stop call that will close the
	// logger.
	h.log, err = dependencies.NewLogger(filepath.Join(h.persistDir, logFile))
	if err != nil {
		errChan <- fmt.Errorf("error creating logger: %w", err)
		return nil, errChan
	}

	h.tg.AfterStop(func() {
		err = h.log.Close()
		if err != nil {
			// State of the logger is uncertain, a Println will have to
			// suffice.
			fmt.Println("Error when closing the logger:", err)
		}
	})

	go func() {
		defer close(errChan)

		err := h.delayedHostInitialization(smDeps, cs, listenerAddress, persistDir, hostAPIListener, checkTokenExpirationFrequency, onlyFirstDir)
		if err != nil {
			errChan <- err
		}
	}()

	//remove obsoleted ephemeral accounts and fingerprintsbucket files if there are any
	h.log.Debugf("Removing fingerprintsbucket files from %v", h.persistDir)
	err = removeObsoletedFiles(h.persistDir)
	if err != nil {
		h.log.Debugf("remove fingerprintsbucket files failed: %v", err)
	}
	return h, errChan
}

func (h *Host) delayedHostInitialization(smDeps modules.Dependencies, cs modules.ConsensusSet, listenerAddress, persistDir string, hostAPIListener net.Listener, checkTokenExpirationFrequency time.Duration, onlyFirstDir bool) error {
	h.log.Debugln("Pausing startup until consensus synced")
	for !cs.Synced() {
		h.log.Debugln("Consensus not synced yet")
		select {
		case <-h.tg.StopChan():
			h.log.Debugln("Interrupted by shutdown")
			return nil
		case <-time.After(1 * time.Second):
			//check cs.Synced() again
		}
	}
	h.log.Debugln("Consensus synced")

	tokenStorageDir := filepath.Join(h.persistDir, tokenStorDir)
	if _, err := os.Stat(tokenStorageDir); os.IsNotExist(err) {
		// Create the token storage directory if it does not yet exist.
		if err = os.Mkdir(tokenStorageDir, 0755); err != nil {
			return fmt.Errorf("error creating token storage directory: %w", err)
		}
	}
	h.log.Debugf("Token storage directory (%v) ready", tokenStorageDir)
	// Add the storage manager to the host, and set up the stop call that will
	// close the storage manager.
	stManager, err := contractmanager.NewCustomContractManager(smDeps, filepath.Join(persistDir, "contractmanager"), onlyFirstDir)
	if err != nil {
		h.log.Println("Could not open the storage manager:", err)
		return fmt.Errorf("error creating contract manager: %w", err)
	}
	h.StorageManager = stManager
	h.log.Debugf("Contract manager ready")
	h.tg.AfterStop(func() {
		err = h.StorageManager.Close()
		if err != nil {
			h.log.Println("Could not close storage manager:", err)
		}
	})

	// Initialize token storage.
	// TODO: the current version of tokens storage does not support reverting blocks.
	// If contracts related to `TopUpToken` RPC are reverted, all the tokens resources remain in the storage.
	h.tokenStor, err = tokenstorage.NewTokenStorage(stManager, tokenStorageDir)
	if err != nil {
		return fmt.Errorf("error initializing token storage: %w", err)
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
		return fmt.Errorf("error loading persistence: %w", err)
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
		return fmt.Errorf("error subscribing to consensus: %w", err)
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
		return fmt.Errorf("could not initialize host networking: %w", err)
	}

	// Initialize and run host API
	hostApi := api.NewAPI(h.tokenStor, h.secretKey, h)
	err = hostApi.Start(hostAPIListener)
	if err != nil {
		err = fmt.Errorf("error starting host api: %w", err)
		h.log.Errorln(err)
		return err
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
	return nil
}

// NewBlockedStartHost creates and starts host module waiting for consensus
// sync before subscribing to consensus and starting the API.
// Returns host and error channel to catch startup errors
func NewBlockedStartHost(deps modules.Dependencies, smDeps modules.Dependencies,
	cs modules.ConsensusSet, g modules.Gateway, tpool modules.TransactionPool,
	wallet modules.Wallet, address, persistDir string, hostAPIListener net.Listener,
	checkTokenExpirationFrequency time.Duration, onlyFirstDir bool) (*Host, <-chan error) {
	return newHostBlocked(deps, smDeps, cs, g, tpool, wallet, address, persistDir, hostAPIListener, checkTokenExpirationFrequency, onlyFirstDir)
}
