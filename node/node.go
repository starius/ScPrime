// Package node provides tooling for creating a Sia node. Sia nodes consist of a
// collection of modules. The node package gives you tools to easily assemble
// various combinations of modules with varying dependencies and settings,
// including templates for assembling sane no-hassle Sia nodes.
package node

// TODO: Add support for the explorer.

// TODO: Add support for custom dependencies and parameters for all of the
// modules.

import (
	"fmt"
	"path/filepath"
	"time"

	"gitlab.com/NebulousLabs/errors"

	"gitlab.com/SiaPrime/SiaPrime/build"
	"gitlab.com/SiaPrime/SiaPrime/config"
	"gitlab.com/SiaPrime/SiaPrime/modules"
	"gitlab.com/SiaPrime/SiaPrime/modules/consensus"
	"gitlab.com/SiaPrime/SiaPrime/modules/explorer"
	"gitlab.com/SiaPrime/SiaPrime/modules/gateway"
	"gitlab.com/SiaPrime/SiaPrime/modules/host"
	"gitlab.com/SiaPrime/SiaPrime/modules/miner"
	"gitlab.com/SiaPrime/SiaPrime/modules/miningpool"
	"gitlab.com/SiaPrime/SiaPrime/modules/renter"
	"gitlab.com/SiaPrime/SiaPrime/modules/renter/contractor"
	"gitlab.com/SiaPrime/SiaPrime/modules/renter/hostdb"
	"gitlab.com/SiaPrime/SiaPrime/modules/renter/proto"
	"gitlab.com/SiaPrime/SiaPrime/modules/stratumminer"
	"gitlab.com/SiaPrime/SiaPrime/modules/transactionpool"
	"gitlab.com/SiaPrime/SiaPrime/modules/wallet"
	"gitlab.com/SiaPrime/SiaPrime/persist"
)

// NodeParams contains a bunch of parameters for creating a new test node. As
// there are many options, templates are provided that you can modify which
// cover the most common use cases.
//
// Each module is created separately. There are several ways to create a module,
// though not all methods are currently available for each module. You should
// only use one method for creating a module, using multiple methods will cause
// an error.
//		+ Indicate with the 'CreateModule' bool that a module should be created
//		  automatically. To create the module with custom dependencies, pass the
//		  custom dependencies in using the 'ModuleDependencies' field.
//		+ Pass an existing module in directly.
//		+ Set 'CreateModule' to false and do not pass in an existing module.
//		  This will result in a 'nil' module, meaning the node will not have
//		  that module.
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
	ContractorDeps  modules.Dependencies
	ContractSetDeps modules.Dependencies
	HostDBDeps      modules.Dependencies
	RenterDeps      modules.Dependencies
	WalletDeps      modules.Dependencies

	// Custom settings for modules
	Allowance   modules.Allowance
	Bootstrap   bool
	HostAddress string
	HostStorage uint64
	RPCAddress  string

	// Initialize node from existing seed.
	PrimarySeed string

	// The following fields are used to skip parts of the node set up
	SkipSetAllowance     bool
	SkipHostDiscovery    bool
	SkipHostAnnouncement bool

	// The high level directory where all the persistence gets stored for the
	// modules.
	Dir string
}

// Node is a collection of Sia modules operating together as a Sia node.
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
	if !np.CreateExplorer || np.Explorer != nil {
		n++
	}
	if !np.CreateMiningPool || np.MiningPool != nil {
		n++
	}
	if !np.CreateStratumMiner || np.StratumMiner != nil {
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
	if n.MiningPool != nil {
		printlnRelease("Closing mining pool...")
		err = errors.Compose(n.MiningPool.Close())
	}
	if n.StratumMiner != nil {
		printlnRelease("Closing stratum miner...")
		err = errors.Compose(n.StratumMiner.Close())
	}
	return err
}

// New will create a new test node. The inputs to the function are the
// respective 'New' calls for each module. We need to use this awkward method
// of initialization because the siatest package cannot import any of the
// modules directly (so that the modules may use the siatest package to test
// themselves).
func New(params NodeParams) (*Node, error) {
	dir := params.Dir
	numModules := params.NumModules()
	i := 1
	printfRelease("(%d/%d) Loading spd...\n", i, numModules)
	loadStart := time.Now()

	// Gateway.
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
			params.RPCAddress = "localhost:0"
		}
		i++
		printfRelease("(%d/%d) Loading gateway...", i, numModules)
		return gateway.New(params.RPCAddress, params.Bootstrap, filepath.Join(dir, modules.GatewayDir))
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create gateway"))
	}
	printlnRelease(" done in ", time.Since(loadStart).Seconds(), "seconds.")
	loadStart = time.Now()
	// Consensus.
	cs, err := func() (modules.ConsensusSet, error) {
		if params.CreateConsensusSet && params.ConsensusSet != nil {
			return nil, errors.New("cannot both create consensus and use passed in consensus")
		}
		if params.ConsensusSet != nil {
			return params.ConsensusSet, nil
		}
		if !params.CreateConsensusSet {
			return nil, nil
		}
		i++
		printfRelease("(%d/%d) Loading consensus...", i, numModules)
		return consensus.New(g, params.Bootstrap, filepath.Join(dir, modules.ConsensusDir))
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create consensus set"))
	}
	printlnRelease(" done in ", time.Since(loadStart).Seconds(), "seconds.")
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
		printfRelease("(%d/%d) Loading explorer...", i, numModules)
		e, err := explorer.New(cs, filepath.Join(dir, modules.ExplorerDir))
		if err != nil {
			return nil, err
		}
		return e, nil
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create explorer"))
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
		return transactionpool.New(cs, g, filepath.Join(dir, modules.TransactionPoolDir))
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create transaction pool"))
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
		i++
		printfRelease("(%d/%d) Loading wallet...", i, numModules)
		return wallet.NewCustomWallet(cs, tp, filepath.Join(dir, modules.WalletDir), walletDeps)
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create wallet"))
	}
	if w != nil {
		printlnRelease(" done in ", time.Since(loadStart).Seconds(), "seconds.")
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
		return nil, errors.Extend(err, errors.New("unable to create miner"))
	}
	if m != nil {
		printlnRelease(" done in ", time.Since(loadStart).Seconds(), "seconds.")
	}
	loadStart = time.Now()

	// Host.
	h, err := func() (modules.Host, error) {
		if params.CreateHost && params.Host != nil {
			return nil, errors.New("cannot create host and use custom host")
		}
		if params.Host != nil {
			return params.Host, nil
		}
		if !params.CreateHost {
			return nil, nil
		}
		if params.HostAddress == "" {
			params.HostAddress = "localhost:0"
		}
		i++
		printfRelease("(%d/%d) Loading host...", i, numModules)
		return host.New(cs, g, tp, w, params.HostAddress, filepath.Join(dir, modules.HostDir))
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create host"))
	}
	if h != nil {
		printlnRelease(" done in ", time.Since(loadStart).Seconds(), "seconds.")
	}
	loadStart = time.Now()

	// Renter.
	r, err := func() (modules.Renter, error) {
		if params.CreateRenter && params.Renter != nil {
			return nil, errors.New("cannot create renter and also use custom renter")
		}
		if params.Renter != nil {
			return params.Renter, nil
		}
		if !params.CreateRenter {
			return nil, nil
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
		hdb, err := hostdb.NewCustomHostDB(g, cs, tp, persistDir, hostDBDeps)
		if err != nil {
			return nil, err
		}

		// ContractSet
		contractSet, err := proto.NewContractSet(filepath.Join(persistDir, "contracts"), contractSetDeps)
		if err != nil {
			return nil, err
		}

		// Contractor
		logger, err := persist.NewFileLogger(filepath.Join(persistDir, "contractor.log"))
		if err != nil {
			return nil, err
		}
		contractorWallet := &contractor.WalletBridge{W: w}
		contractorPersist := contractor.NewPersist(persistDir)
		hc, err := contractor.NewCustomContractor(cs, contractorWallet, tp, hdb, contractSet, contractorPersist, logger, contractorDeps)
		if err != nil {
			logger.Debugln("Renter start aborted, error starting contractor.")
			return nil, err
		}
		return renter.NewCustomRenter(g, cs, tp, hdb, w, hc, persistDir, renterDeps)
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create renter"))
	}
	if r != nil {
		printlnRelease(" done in ", time.Since(loadStart).Seconds(), "seconds.")
	}
	loadStart = time.Now()

	// Mining Pool.
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
		p, err := pool.New(cs, tp, g, w, filepath.Join(dir, modules.PoolDir), config.MiningPoolConfig{})
		if err != nil {
			return nil, err
		}
		return p, nil
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create mining pool"))
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
		printfRelease("(%d/%d) Loading stratum miner...", i, numModules)
		sm, err := stratumminer.New(filepath.Join(dir, modules.StratumMinerDir))
		if err != nil {
			return nil, err
		}
		return sm, nil
	}()
	if err != nil {
		return nil, errors.Extend(err, errors.New("unable to create stratumminer"))
	}
	if sm != nil {
		printlnRelease(" done in ", time.Since(loadStart).Seconds(), "seconds.")
	}

	return &Node{
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
	}, nil
}
