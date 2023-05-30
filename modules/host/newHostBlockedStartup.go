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
	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/host/api"
	"gitlab.com/scpcorp/ScPrime/modules/host/contractmanager"
	"gitlab.com/scpcorp/ScPrime/modules/host/tokenstorage"
	"gitlab.com/scpcorp/ScPrime/types"
)

// newHostBlocked returns a Host
// but delays the loading and starting until the consensus is synced and wallet unlocked
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

	//Catch stop signal during startup
	abortChan := make(chan int)
	h.tg.OnStop(func() {
		h.log.Debugln("Stop during initializer waiting for consensus")
		close(abortChan)
	})

	//Block consensus subscription until consensus and wallet ready
	h.log.Debugln("Pausing startup until consensus synced and wallet unlocked")
	// open waitchan
	// Send true if consensus ready and wallet unlocked or false if wallet is not initialized
	blocker := make(chan bool)

	//Start unblocker func
	go func() {
		// If it is fresh node wallet is not initialized yet so will
		// never unlock. Skip blocker in that case and tell host that there is no wallet
		walletExists, _ := wallet.Encrypted()
		if !walletExists {
			blocker <- false
			return
		}
		//Wait for consensus synced
		for !cs.Synced() {
			select {
			case <-abortChan:
				//abort due to shutdown
				return
			case <-time.After(1 * time.Second):
				//check cs.Synced() again
			}
		}
		//consensus synced here, wait for wallet unlocked
		if build.Release == "testing" {
			h.log.Debugln("wallet unlock blocker cancelled for testing speedup")
			blocker <- true
			return
		}
		for unlocked, serr := wallet.Unlocked(); !unlocked; unlocked, serr = wallet.Unlocked() {
			if serr != nil {
				h.log.Debugf("startup blocker error checking wallet: %v", err.Error())
			}
			select {
			case <-abortChan:
				//abort due to shutdown
				return
			case <-time.After(1 * time.Second):
				//check wallet.Unlocked() again
			}
		}
		//Wallet unlocked, unblock host loader
		blocker <- true
	}()

	//Start host initializer blocked
	go func() {
		defer close(errChan)
		var walletReady bool
		select {
		case <-abortChan:
			//abort due to shutdown
			return
		case walletReady = <-blocker:
			// Unblock signal received, continue host initialization
			if walletReady {
				h.log.Debugln("Resuming host module loading after consensus synced and wallet unlocked")
			} else {
				h.log.Debugln("Resuming host module loading without wallet")
			}
		}
		//TODO: There is no reason to fully initialize if there is no wallet

		// Add the storage manager to the host, and set up the stop call that will
		// close the storage manager.
		stManager, err := contractmanager.NewCustomContractManager(smDeps, filepath.Join(persistDir, "contractmanager"), onlyFirstDir)
		if err != nil {
			h.log.Println("Could not open the storage manager:", err)
			errChan <- fmt.Errorf("error creating contract manager: %w", err)
			return
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

		tokenStorageDir := filepath.Join(h.persistDir, tokenStorDir)
		if _, err = os.Stat(tokenStorageDir); os.IsNotExist(err) {
			// Create the token storage directory if it does not yet exist.
			if err = os.Mkdir(tokenStorageDir, 0755); err != nil {
				errChan <- fmt.Errorf("error creating token storage directory: %w", err)
				return
			}
		}
		h.log.Debugf("Token storage directory (%v) ready", tokenStorageDir)

		// Initialize token storage.
		// TODO: the current version of tokens storage does not support reverting blocks.
		// If contracts related to `TopUpToken` RPC are reverted, all the tokens resources remain in the storage.
		h.tokenStor, err = tokenstorage.NewTokenStorage(stManager, tokenStorageDir)
		if err != nil {
			errChan <- fmt.Errorf("error initializing token storage: %w", err)
			return
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
			errChan <- fmt.Errorf("error loading persistence: %w", err)
			return
		}
		h.log.Debugf("Host persistence loaded")
		h.tg.AfterStop(func() {
			err = h.saveSync()
			if err != nil {
				h.log.Errorf("Error saving host upon shutdown: %v", err)
			}
		})

		// Create bandwidth monitor
		h.staticMonitor = connmonitor.NewMonitor()

		// Initialize the networking.
		err = h.initNetworking(listenerAddress)
		if err != nil {
			h.log.Errorf("Could not initialize host networking: %v", err)
			errChan <- fmt.Errorf("could not initialize host networking: %w", err)
			return
		}

		//	Initialize and run host API
		hostApi := api.NewAPI(h.tokenStor, h.secretKey, h)
		err = hostApi.Start(hostAPIListener)
		if err != nil {
			err = fmt.Errorf("error starting host api: %w", err)
			h.log.Errorln(err)
			errChan <- err
			return
		}
		h.tg.AfterStop(func() {
			err = hostApi.Close()
			if err != nil {
				h.log.Errorln("Could not close host API:", err)
				err = fmt.Errorf("error closing host API: %w", err)
			}
		})

		atomic.StoreUint64(&h.scheduledAuditBlockheight, uint64(h.blockHeight))
		h.log.Debugf("scheduled audit blockheight set to %v", h.blockHeight)
		// Subscribe to the consensus set.
		err = h.initConsensusSubscription()
		if err != nil {
			errChan <- fmt.Errorf("error subscribing to consensus: %w", err)
			return
		}
		h.log.Debugln("Consensus subscription initialized")
		h.atomicReadyToServe.Store(true)
	}()

	//remove obsoleted ephemeral accounts and fingerprintsbucket files if there are any
	h.log.Debugf("Removing fingerprintsbucket files from %v", h.persistDir)
	err = removeObsoletedFiles(h.persistDir)
	if err != nil {
		h.log.Debugf("remove fingerprintsbucket files failed: %v", err)
	}
	return h, errChan
}

// NewBlockedStartHost creates and starts host module waiting for consensus
// sync and wallet unlock before subscribing to consensus and starting the API.
// Returns host and error channel to catch startup errors
func NewBlockedStartHost(deps modules.Dependencies, smDeps modules.Dependencies,
	cs modules.ConsensusSet, g modules.Gateway, tpool modules.TransactionPool,
	wallet modules.Wallet, address, persistDir string, hostAPIListener net.Listener,
	checkTokenExpirationFrequency time.Duration, onlyFirstDir bool) (*Host, <-chan error) {
	return newHostBlocked(deps, smDeps, cs, g, tpool, wallet, address, persistDir, hostAPIListener, checkTokenExpirationFrequency, onlyFirstDir)
}
