// Package node provides tooling for creating a ScPrime node. ScPrime nodes consist of a
// collection of modules. The node package gives you tools to easily assemble
// various combinations of modules with varying dependencies and settings,
// including templates for assembling sane no-hassle ScPrime nodes.
package node

// TODO: Add support for the explorer.

// TODO: Add support for custom dependencies and parameters for all of the
// modules.

import (
	"fmt"
	"net"
	"path/filepath"
	"time"

	"gitlab.com/scpcorp/spf-transporter"

	mnemonics "gitlab.com/NebulousLabs/entropy-mnemonics"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/ratelimit"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/config"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/consensus"
	"gitlab.com/scpcorp/ScPrime/modules/explorer"
	"gitlab.com/scpcorp/ScPrime/modules/gateway"
	"gitlab.com/scpcorp/ScPrime/modules/host"
	"gitlab.com/scpcorp/ScPrime/modules/miner"
	pool "gitlab.com/scpcorp/ScPrime/modules/miningpool"
	"gitlab.com/scpcorp/ScPrime/modules/renter"
	"gitlab.com/scpcorp/ScPrime/modules/renter/contractor"
	"gitlab.com/scpcorp/ScPrime/modules/renter/hostdb"
	"gitlab.com/scpcorp/ScPrime/modules/renter/proto"
	"gitlab.com/scpcorp/ScPrime/modules/stratumminer"
	"gitlab.com/scpcorp/ScPrime/modules/transactionpool"
	"gitlab.com/scpcorp/ScPrime/modules/wallet"
	"gitlab.com/scpcorp/ScPrime/persist"
)

// NodeParams contains a bunch of parameters for creating a new test node. As
// there are many options, templates are provided that you can modify which
// cover the most common use cases.
//
// Each module is created separately. There are several ways to create a module,
// though not all methods are currently available for each module. You should
// only use one method for creating a module, using multiple methods will cause
// an error.
//   - Indicate with the 'CreateModule' bool that a module should be created
//     automatically. To create the module with custom dependencies, pass the
//     custom dependencies in using the 'ModuleDependencies' field.
//   - Pass an existing module in directly.
//   - Set 'CreateModule' to false and do not pass in an existing module.
//     This will result in a 'nil' module, meaning the node will not have
//     that module.
type NodeParams struct {
	// Flags to indicate which modules should be created automatically by the
	// server. If you are providing a pre-existing module, do not set the flag
	// for that module.
	//
	// NOTE / TODO: The code does not currently enforce this, but you should not
	// provide a custom module unless all of its dependencies are also custom.
	// Example: if the ConsensusSet is custom, the Gateway should also be
	// custom. The TransactionPool however does not need to be custom in this
	// example.
	CreateConsensusSet    bool
	CreateExplorer        bool
	CreateGateway         bool
	CreateHost            bool
	CreateMiner           bool
	CreateMiningPool      bool
	CreateStratumMiner    bool
	CreateRenter          bool
	CreateTransactionPool bool
	CreateWallet          bool

	// Custom modules - if the modules is provided directly, the provided
	// module will be used instead of creating a new one. If a custom module is
	// provided, the 'omit' flag for that module must be set to false (which is
	// the default setting).
	ConsensusSet    modules.ConsensusSet
	Explorer        modules.Explorer
	Gateway         modules.Gateway
	Host            modules.Host
	Miner           modules.TestMiner
	MiningPool      modules.Pool
	StratumMiner    modules.StratumMiner
	Renter          modules.Renter
	TransactionPool modules.TransactionPool
	Wallet          modules.Wallet

	// Dependencies for each module supporting dependency injection.
	ConsensusSetDeps modules.Dependencies
	ContractorDeps   modules.Dependencies
	ContractSetDeps  modules.Dependencies
	GatewayDeps      modules.Dependencies
	HostDeps         modules.Dependencies
	HostDBDeps       modules.Dependencies
	RenterDeps       modules.Dependencies
	TPoolDeps        modules.Dependencies
	WalletDeps       modules.Dependencies

	// Dependencies for storage monitor supporting dependency injection.
	StorageManagerDeps modules.Dependencies

	// Custom settings for modules
	Allowance   modules.Allowance
	Bootstrap   bool
	HostAddress string
	HostStorage uint64
	RPCAddress  string

	// Address of the SPF transporter for wallet module.
	SpfTransporterAddress string

	// Initialize node from existing seed.
	PrimarySeed string

	// The following fields are used to skip parts of the node set up
	SkipSetAllowance     bool
	SkipHostDiscovery    bool
	SkipHostAnnouncement bool
	SkipWalletInit       bool

	// The high level directory where all the persistence gets stored for the
	// modules.
	Dir string

	// Configuration settings for the Mining pool.
	PoolConfig config.MiningPoolConfig

	HostAPIAddr                   string
	CheckTokenExpirationFrequency time.Duration
	OnlyFirstDir                  bool
}

// Node is a collection of ScPrime modules operating together as a ScPrime node.
type Node struct {
	// The modules of the node. Modules that are not initialized will be nil.
	ConsensusSet    modules.ConsensusSet
	Explorer        modules.Explorer
	Gateway         modules.Gateway
	Host            modules.Host
	Miner           modules.TestMiner
	MiningPool      modules.Pool
	StratumMiner    modules.StratumMiner
	Renter          modules.Renter
	TransactionPool modules.TransactionPool
	Wallet          modules.Wallet

	// The high level directory where all the persistence gets stored for the
	// modules.
	Dir string
}

// NumModules returns how many of the major modules the given NodeParams would
// create.
func (np NodeParams) NumModules() (n int) {
	if np.CreateGateway || np.Gateway != nil {
		n++
	}
	if np.CreateConsensusSet || np.ConsensusSet != nil {
		n++
	}
	if np.CreateTransactionPool || np.TransactionPool != nil {
		n++
	}
	if np.CreateWallet || np.Wallet != nil {
		n++
	}
	if np.CreateHost || np.Host != nil {
		n++
	}
	if np.CreateRenter || np.Renter != nil {
		n++
	}
	if np.CreateMiner || np.Miner != nil {
		n++
	}
	if np.CreateExplorer || np.Explorer != nil {
		n++
	}
	if np.CreateMiningPool || np.MiningPool != nil {
		n++
	}
	if np.CreateStratumMiner || np.StratumMiner != nil {
		n++
	}
	return
}

// printlnRelease is a wrapper that only prints to stdout in release builds.
func printlnRelease(a ...interface{}) (int, error) {
	if build.Release == "standard" {
		return fmt.Println(a...)
	}
	return 0, nil
}

// printfRelease is a wrapper that only prints to stdout in release builds.
func printfRelease(format string, a ...interface{}) (int, error) {
	if build.Release == "standard" {
		return fmt.Printf(format, a...)
	}
	return 0, nil
}

// Close will call close on every module within the node, combining and
// returning the errors.
func (n *Node) Close() (err error) {
	if n.MiningPool != nil {
		printlnRelease("Closing mining pool...")
		err = errors.Compose(n.MiningPool.Close())
	}
	if n.StratumMiner != nil {
		printlnRelease("Closing stratum miner...")
		err = errors.Compose(n.StratumMiner.Close())
	}
	if n.Renter != nil {
		printlnRelease("Closing renter...")
		err = errors.Compose(n.Renter.Close())
	}
	if n.Host != nil {
		printlnRelease("Closing host...")
		err = errors.Compose(n.Host.Close())
	}
	if n.Miner != nil {
		printlnRelease("Closing miner...")
		err = errors.Compose(n.Miner.Close())
	}
	if n.Wallet != nil {
		printlnRelease("Closing wallet...")
		err = errors.Compose(n.Wallet.Close())
	}
	if n.TransactionPool != nil {
		printlnRelease("Closing transactionpool...")
		err = errors.Compose(n.TransactionPool.Close())
	}
	if n.Explorer != nil {
		printlnRelease("Closing explorer...")
		err = errors.Compose(n.Explorer.Close())
	}
	if n.ConsensusSet != nil {
		printlnRelease("Closing consensusset...")
		err = errors.Compose(n.ConsensusSet.Close())
	}
	if n.Gateway != nil {
		printlnRelease("Closing gateway...")
		err = errors.Compose(n.Gateway.Close())
	}
	return err
}

// New will create a new node. The inputs to the function are the respective
// 'New' calls for each module. We need to use this awkward method of
// initialization because the siatest package cannot import any of the modules
// directly (so that the modules may use the siatest package to test
// themselves).
func New(params NodeParams, loadStartTime time.Time) (*Node, <-chan error) {
	walletUnlocked := false
	numModules := params.NumModules()
	i := 0
	printlnRelease("Starting modules:")

	// Make sure the path is an absolute one.
	dir, err := filepath.Abs(params.Dir)
	errChan := make(chan error, 1)
	if err != nil {
		errChan <- err
		return nil, errChan
	}

	// Gateway.
	loadStart := time.Now()
	g, err := func() (modules.Gateway, error) {
		if params.CreateGateway && params.Gateway != nil {
			return nil, errors.New("cannot both create a gateway and use a passed in gateway")
		}
		if params.Gateway != nil {
			return params.Gateway, nil
		}
		if !params.CreateGateway {
			return nil, nil
		}
		if params.RPCAddress == "" {
			params.RPCAddress = "127.0.0.1:0"
		}
		gatewayDeps := params.GatewayDeps
		if gatewayDeps == nil {
			gatewayDeps = modules.ProdDependencies
		}
		i++
		printfRelease("(%d/%d) Loading gateway...", i, numModules)
		return gateway.NewCustomGateway(params.RPCAddress, params.Bootstrap, filepath.Join(dir, modules.GatewayDir), gatewayDeps)
	}()
	if err != nil {
		errChan <- errors.Extend(err, errors.New("unable to create gateway"))
		return nil, errChan
	}
	if g != nil {
		printlnRelease(" done in ", time.Since(loadStart).Seconds(), "seconds.")
	}

	loadStart = time.Now()
	// Consensus.
	cs, errChanCS := func() (modules.ConsensusSet, <-chan error) {
		c := make(chan error, 1)
		defer close(c)
		if params.CreateConsensusSet && params.ConsensusSet != nil {
			c <- errors.New("cannot both create consensus and use passed in consensus")
			return nil, c
		}
		if params.ConsensusSet != nil {
			return params.ConsensusSet, c
		}
		if !params.CreateConsensusSet {
			return nil, c
		}
		i++
		printfRelease("(%d/%d) Loading consensus...", i, numModules)
		consensusSetDeps := params.ConsensusSetDeps
		if consensusSetDeps == nil {
			consensusSetDeps = modules.ProdDependencies
		}
		return consensus.NewCustomConsensusSet(g, params.Bootstrap, filepath.Join(dir, modules.ConsensusDir), consensusSetDeps)
	}()
	if err := modules.PeekErr(errChanCS); err != nil {
		errChan <- errors.Extend(err, errors.New("unable to create consensus set"))
		return nil, errChan
	}
	if cs != nil {
		printlnRelease(" done in ", time.Since(loadStart).Seconds(), "seconds.")
	}

	loadStart = time.Now()
	// Explorer.
	e, err := func() (modules.Explorer, error) {
		if !params.CreateExplorer && params.Explorer != nil {
			return nil, errors.New("cannot create explorer and also use custom explorer")
		}
		if params.Explorer != nil {
			return params.Explorer, nil
		}
		if !params.CreateExplorer {
			return nil, nil
		}
		i++
		printfRelease("(%d/%d) Loading explorer... ", i, numModules)
		e, err := explorer.New(cs, filepath.Join(dir, modules.ExplorerDir))
		if err != nil {
			return nil, err
		}
		return e, nil
	}()
	if err != nil {
		errChan <- errors.Extend(err, errors.New("unable to create explorer"))
		return nil, errChan
	}
	if e != nil {
		printlnRelease(" done in ", time.Since(loadStart).Seconds(), "seconds.")
	}

	loadStart = time.Now()
	// Transaction Pool.
	tp, err := func() (modules.TransactionPool, error) {
		if params.CreateTransactionPool && params.TransactionPool != nil {
			return nil, errors.New("cannot create transaction pool and also use custom transaction pool")
		}
		if params.TransactionPool != nil {
			return params.TransactionPool, nil
		}
		if !params.CreateTransactionPool {
			return nil, nil
		}
		i++
		printfRelease("(%d/%d) Loading transaction pool...", i, numModules)
		tpoolDeps := params.TPoolDeps
		if tpoolDeps == nil {
			tpoolDeps = modules.ProdDependencies
		}
		return transactionpool.NewCustomTPool(cs, g, filepath.Join(dir, modules.TransactionPoolDir), tpoolDeps)
	}()
	if err != nil {
		errChan <- errors.Extend(err, errors.New("unable to create transaction pool"))
		return nil, errChan
	}
	if tp != nil {
		printlnRelease(" done in ", time.Since(loadStart).Seconds(), "seconds.")
	}

	loadStart = time.Now()
	// Wallet.
	w, err := func() (modules.Wallet, error) {
		if params.CreateWallet && params.Wallet != nil {
			return nil, errors.New("cannot create wallet and use custom wallet")
		}
		if params.Wallet != nil {
			return params.Wallet, nil
		}
		if !params.CreateWallet {
			return nil, nil
		}
		walletDeps := params.WalletDeps
		if walletDeps == nil {
			walletDeps = modules.ProdDependencies
		}
		var tc wallet.TransporterClient
		if params.SpfTransporterAddress != "" {
			tc, err = transporter.NewClient(params.SpfTransporterAddress)
			if err != nil {
				return nil, errors.Extend(err, errors.New("unable to create SPF transporter client for wallet"))
			}
		}
		i++
		printfRelease("(%d/%d) Loading wallet...", i, numModules)
		return wallet.NewCustomWallet(cs, tp, filepath.Join(dir, modules.WalletDir), walletDeps, wallet.WithTransporterClient(tc))
	}()
	if err != nil {
		errChan <- errors.Extend(err, errors.New("unable to create wallet"))
		return nil, errChan
	}
	if w != nil {
		printlnRelease(" done in ", time.Since(loadStart).Seconds(), "seconds.")
		//Start wallet unlock attempt
		if password := build.WalletPassword(); password != "" {
			// fmt.Println("ScPrime Wallet Password found, attempting to auto-unlock wallet")
			go func() {
				var validKeys []crypto.CipherKey
				dicts := []mnemonics.DictionaryID{"english", "german", "japanese"}
				for _, dict := range dicts {
					seed, err := modules.StringToSeed(password, dict)
					if err != nil {
						continue
					}
					validKeys = append(validKeys, crypto.NewWalletKey(crypto.HashObject(seed)))
				}
				validKeys = append(validKeys, crypto.NewWalletKey(crypto.HashObject(password)))
				for _, key := range validKeys {
					if err := w.Unlock(key); err == nil {
						walletUnlocked = true
						return
					}
				}
			}()
		}
	}

	loadStart = time.Now()
	// Miner.
	m, err := func() (modules.TestMiner, error) {
		if params.CreateMiner && params.Miner != nil {
			return nil, errors.New("cannot create miner and also use custom miner")
		}
		if params.Miner != nil {
			return params.Miner, nil
		}
		if !params.CreateMiner {
			return nil, nil
		}
		i++
		printfRelease("(%d/%d) Loading miner...", i, numModules)
		m, err := miner.New(cs, tp, w, filepath.Join(dir, modules.MinerDir))
		if err != nil {
			return nil, err
		}
		return m, nil
	}()
	if err != nil {
		errChan <- errors.Extend(err, errors.New("unable to create miner"))
		return nil, errChan
	}
	if m != nil {
		printlnRelease(" done in ", time.Since(loadStart).Seconds(), "seconds.")
	}

	loadStart = time.Now()
	// Host.
	h, errChanHost := func() (modules.Host, <-chan error) {
		c := make(chan error, 1)
		defer close(c)
		if params.CreateHost && params.Host != nil {
			c <- errors.New("cannot create host and use custom host")
			return nil, c
		}
		if params.Host != nil {
			return params.Host, c
		}
		if !params.CreateHost {
			return nil, c
		}
		if params.HostAddress == "" {
			params.HostAddress = "127.0.0.1:0"
		}
		hostDeps := params.HostDeps
		if hostDeps == nil {
			hostDeps = modules.ProdDependencies
		}
		smDeps := params.StorageManagerDeps
		if smDeps == nil {
			smDeps = new(modules.ProductionDependencies)
		}

		ln, err := net.Listen("tcp", params.HostAPIAddr)
		if err != nil {
			c <- fmt.Errorf("error creating network listener for host api: %w", err)
			return nil, c
		}

		i++
		printfRelease("(%d/%d) Loading host...", i, numModules)
		return host.NewBlockedStartHost(hostDeps, smDeps, cs, g, tp, w, params.HostAddress, filepath.Join(dir, modules.HostDir), ln, params.CheckTokenExpirationFrequency, params.OnlyFirstDir)
	}()
	if err := modules.PeekErr(errChanHost); err != nil {
		errChan <- fmt.Errorf("unable to initialize host module: %w", err)
		return nil, errChan
	}
	if h != nil {
		printlnRelease(" done in ", time.Since(loadStart).Seconds(), "seconds.")
	}

	loadStart = time.Now()
	// Renter.
	r, errChanRenter := func() (modules.Renter, <-chan error) {
		c := make(chan error, 1)
		if params.CreateRenter && params.Renter != nil {
			c <- errors.New("cannot create renter and also use custom renter")
			close(c)
			return nil, c
		}
		if params.Renter != nil {
			close(c)
			return params.Renter, c
		}
		if !params.CreateRenter {
			close(c)
			return nil, c
		}
		contractorDeps := params.ContractorDeps
		if contractorDeps == nil {
			contractorDeps = modules.ProdDependencies
		}
		contractSetDeps := params.ContractSetDeps
		if contractSetDeps == nil {
			contractSetDeps = modules.ProdDependencies
		}
		hostDBDeps := params.HostDBDeps
		if hostDBDeps == nil {
			hostDBDeps = modules.ProdDependencies
		}
		renterDeps := params.RenterDeps
		if renterDeps == nil {
			renterDeps = modules.ProdDependencies
		}
		persistDir := filepath.Join(dir, modules.RenterDir)

		i++
		printfRelease("(%d/%d) Loading renter...", i, numModules)

		// HostDB
		hdb, errChanHDB := hostdb.NewCustomHostDB(g, cs, tp, persistDir, hostDBDeps)
		if err := modules.PeekErr(errChanHDB); err != nil {
			c <- err
			close(c)
			return nil, c
		}
		// ContractSet
		renterRateLimit := ratelimit.NewRateLimit(0, 0, 0)
		contractSet, err := proto.NewContractSet(filepath.Join(persistDir, "contracts"), renterRateLimit, contractSetDeps)
		if err != nil {
			c <- err
			close(c)
			return nil, c
		}
		// Contractor
		logger, err := persist.NewFileLogger(filepath.Join(persistDir, "contractor.log"))
		if err != nil {
			c <- err
			close(c)
			return nil, c
		}
		hc, errChanContractor := contractor.NewCustomContractor(cs, w, tp, hdb, persistDir, contractSet, logger, contractorDeps)
		if err := modules.PeekErr(errChanContractor); err != nil {
			c <- err
			close(c)
			return nil, c
		}
		renter, errChanRenter := renter.NewCustomRenter(g, cs, tp, hdb, w, hc, persistDir, renterRateLimit, renterDeps)
		if err := modules.PeekErr(errChanRenter); err != nil {
			c <- err
			close(c)
			return nil, c
		}
		go func() {
			c <- errors.Compose(<-errChanHDB, <-errChanContractor, <-errChanRenter)
			close(c)
		}()
		return renter, c
	}()

	if err := modules.PeekErr(errChanRenter); err != nil {
		errChan <- errors.Extend(err, errors.New("unable to create renter"))
		return nil, errChan
	}
	if r != nil {
		printlnRelease(" done in ", time.Since(loadStart).Seconds(), "seconds.")
	}

	// Mining Pool.
	loadStart = time.Now()
	p, err := func() (modules.Pool, error) {
		if params.CreateMiningPool && params.MiningPool != nil {
			return nil, errors.New("cannot create mining pool and also use custom mining pool")
		}
		if params.MiningPool != nil {
			return params.MiningPool, nil
		}
		if !params.CreateMiningPool {
			return nil, nil
		}

		i++
		printfRelease("(%d/%d) Loading mining pool...", i, numModules)
		p, err := pool.New(cs, tp, g, w, filepath.Join(dir, modules.PoolDir), params.PoolConfig)
		if err != nil {
			return nil, err
		}
		return p, nil
	}()
	if err != nil {
		errChan <- errors.Extend(err, errors.New("unable to create mining pool"))
		return nil, errChan
	}
	if p != nil {
		printlnRelease(" done in ", time.Since(loadStart).Seconds(), "seconds.")
	}
	loadStart = time.Now()

	// Stratum Miner.
	sm, err := func() (modules.StratumMiner, error) {
		if params.CreateStratumMiner && params.StratumMiner != nil {
			return nil, errors.New("cannot create stratum miner and also use custom stratum miner")
		}
		if params.StratumMiner != nil {
			return params.StratumMiner, nil
		}
		if !params.CreateStratumMiner {
			return nil, nil
		}
		i++
		printfRelease("(%d/%d) Loading stratum miner...", i, numModules)
		sm, err := stratumminer.New(filepath.Join(dir, modules.StratumMinerDir))
		if err != nil {
			return nil, err
		}
		return sm, nil
	}()
	if err != nil {
		errChan <- errors.Extend(err, errors.New("unable to create stratumminer"))
		return nil, errChan
	}
	if sm != nil {
		printlnRelease(" done in ", time.Since(loadStart).Seconds(), "seconds.")
	}

	printfRelease("API is now available, module loading completed in %.3f seconds\n", time.Since(loadStartTime).Seconds())
	go func() {
		errChan <- errors.Compose(<-errChanCS, <-errChanRenter, <-errChanHost)
		close(errChan)
	}()

	node := &Node{
		ConsensusSet:    cs,
		Explorer:        e,
		Gateway:         g,
		Host:            h,
		Miner:           m,
		MiningPool:      p,
		StratumMiner:    sm,
		Renter:          r,
		TransactionPool: tp,
		Wallet:          w,

		Dir: dir,
	}
	if walletUnlocked {
		printfRelease("Wallet unlocked using environment variable %v\n", build.EnvvarWalletPassword)
	}
	return node, errChan
}
