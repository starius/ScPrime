package renter

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/ratelimit"
	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/consensus"
	"gitlab.com/scpcorp/ScPrime/modules/gateway"
	"gitlab.com/scpcorp/ScPrime/modules/host"
	"gitlab.com/scpcorp/ScPrime/modules/miner"
	"gitlab.com/scpcorp/ScPrime/modules/renter/contractor"
	"gitlab.com/scpcorp/ScPrime/modules/renter/hostdb"
	"gitlab.com/scpcorp/ScPrime/modules/renter/proto"
	"gitlab.com/scpcorp/ScPrime/modules/transactionpool"
	"gitlab.com/scpcorp/ScPrime/modules/wallet"
	"gitlab.com/scpcorp/ScPrime/persist"
	"gitlab.com/scpcorp/ScPrime/types"
)

// renterTester contains all of the modules that are used while testing the renter.
type renterTester struct {
	cs      modules.ConsensusSet
	gateway modules.Gateway
	miner   modules.TestMiner
	tpool   modules.TransactionPool
	wallet  modules.Wallet

	//mux *siamux.SiaMux

	renter *Renter
	dir    string
}

// Close shuts down the renter tester.
func (rt *renterTester) Close() error {
	rt.cs.Close()
	rt.gateway.Close()
	rt.miner.Close()
	rt.tpool.Close()
	rt.wallet.Close()
	//rt.mux.Close()
	rt.renter.Close()
	return nil
}

// addCustomHost adds a host to the test group so that it appears in the host db
func (rt *renterTester) addCustomHost(testdir string, deps modules.Dependencies) (modules.Host, error) {
	h, err := host.NewCustomHost(deps, rt.cs, rt.gateway, rt.tpool, rt.wallet, "127.0.0.1:0", filepath.Join(testdir, modules.HostDir), nil, 5*time.Second)
	if err != nil {
		return nil, errors.AddContext(err, "Failed to create host")
	}

	// configure host to accept contracts
	settings := h.InternalSettings()
	settings.AcceptingContracts = true
	err = h.SetInternalSettings(settings)
	if err != nil {
		return nil, errors.AddContext(err, "Could not set host settings")
	}

	// add storage to host
	storageFolder := filepath.Join(testdir, "storage")
	err = os.MkdirAll(storageFolder, 0700)
	if err != nil {
		return nil, err
	}
	err = h.AddStorageFolder(storageFolder, modules.SectorSize*64)
	if err != nil {
		return nil, errors.AddContext(err, "Could not add folder")
	}

	// announce the host
	err = h.Announce()
	if err != nil {
		return nil, build.ExtendErr("error announcing host", err)
	}

	// mine a block, processing the announcement
	_, err = rt.miner.AddBlock()
	if err != nil {
		return nil, errors.AddContext(err, "Miner can not Add block")
	}

	// wait for hostdb to scan host
	activeHosts, err := rt.renter.ActiveHosts()
	if err != nil {
		return nil, errors.AddContext(err, "Could not get Active hosts")
	}
	for i := 0; i < 50 && len(activeHosts) == 0; i++ {
		time.Sleep(time.Millisecond * 100)
		activeHosts, err = rt.renter.ActiveHosts()
		if err != nil {
			return nil, errors.AddContext(err, "Could not get Active hosts")
		}
	}
	if len(activeHosts) == 0 {
		all, err := rt.renter.AllHosts()
		if err != nil {
			return nil, errors.AddContext(err, "Could not get Active hosts")
		}
		return nil, fmt.Errorf("host did not make it into the contractor hostdb in time, total %v hosts, 0 active\n%+v", len(all), all)
	}

	return h, nil
}

// addHost adds a host to the test group so that it appears in the host db
func (rt *renterTester) addHost(name string) (modules.Host, error) {
	return rt.addCustomHost(filepath.Join(rt.dir, name), modules.ProdDependencies)
}

// addRenter adds a renter to the renter tester and then make sure there is
// money in the wallet
func (rt *renterTester) addRenter(r *Renter) error {
	rt.renter = r
	// Mine blocks until there is money in the wallet.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err := rt.miner.AddBlock()
		if err != nil {
			return err
		}
	}
	return nil
}

// createZeroByteFileOnDisk creates a 0 byte file on disk so that a Stat of the
// local path won't return an error
func (rt *renterTester) createZeroByteFileOnDisk() (string, error) {
	path := filepath.Join(rt.renter.staticFileSystem.Root(), persist.RandomSuffix())
	err := ioutil.WriteFile(path, []byte{}, 0600)
	if err != nil {
		return "", err
	}
	return path, nil
}

// reloadRenter closes the given renter and then re-adds it, effectively
// reloading the renter.
func (rt *renterTester) reloadRenter(r *Renter) (*Renter, error) {
	return rt.reloadRenterWithDependency(r, r.deps)
}

// reloadRenterWithDependency closes the given renter and recreates it using the
// given dependency, it then re-adds the renter on the renter tester effectively
// reloading it.
func (rt *renterTester) reloadRenterWithDependency(r *Renter, deps modules.Dependencies) (*Renter, error) {
	err := r.Close()
	if err != nil {
		return nil, err
	}

	r, err = newRenterWithDependency(rt.gateway, rt.cs, rt.wallet, rt.tpool, filepath.Join(rt.dir, modules.RenterDir), deps)
	if err != nil {
		return nil, err
	}

	err = rt.addRenter(r)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// newRenterTester creates a ready-to-use renter tester with money in the
// wallet.
func newRenterTester(name string) (*renterTester, error) {
	testdir := build.TempDir("renter", name)
	rt, err := newRenterTesterNoRenter(testdir)
	if err != nil {
		return nil, err
	}

	rl := ratelimit.NewRateLimit(0, 0, 0)
	r, errChan := New(rt.gateway, rt.cs, rt.wallet, rt.tpool, rl, filepath.Join(testdir, modules.RenterDir))
	if err := <-errChan; err != nil {
		return nil, err
	}
	err = rt.addRenter(r)
	if err != nil {
		return nil, err
	}
	return rt, nil
}

// newRenterTesterNoRenter creates all the modules for the renter tester except
// the renter. A renter will need to be added and blocks mined to add money to
// the wallet.
func newRenterTesterNoRenter(testdir string) (*renterTester, error) {
	// Create the modules.
	g, err := gateway.New("127.0.0.1:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, errChan := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err := <-errChan; err != nil {
		return nil, err
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		return nil, err
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	key := crypto.GenerateSiaKey(crypto.TypeDefaultWallet)
	_, err = w.Encrypt(key)
	if err != nil {
		return nil, err
	}
	err = w.Unlock(key)
	if err != nil {
		return nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}

	// Assemble all pieces into a renter tester.
	return &renterTester{
		//mux: mux,
		cs:      cs,
		gateway: g,
		miner:   m,
		tpool:   tp,
		wallet:  w,

		dir: testdir,
	}, nil
}

// newRenterTesterWithDependency creates a ready-to-use renter tester with money
// in the wallet.
func newRenterTesterWithDependency(name string, deps modules.Dependencies) (*renterTester, error) {
	testdir := build.TempDir("renter", name)
	rt, err := newRenterTesterNoRenter(testdir)
	if err != nil {
		return nil, err
	}

	// Create the siamux
	// siaMuxDir := filepath.Join(testdir, modules.SiaMuxDir)
	// mux, err := modules.NewSiaMux(siaMuxDir, testdir, "127.0.0.1:0", "127.0.0.1:0")
	// if err != nil {
	// 	return nil, err
	// }

	r, err := newRenterWithDependency(rt.gateway, rt.cs, rt.wallet, rt.tpool, filepath.Join(testdir, modules.RenterDir), deps)
	if err != nil {
		return nil, err
	}
	err = rt.addRenter(r)
	if err != nil {
		return nil, err
	}
	return rt, nil
}

// newRenterWithDependency creates a Renter with custom dependency
func newRenterWithDependency(g modules.Gateway, cs modules.ConsensusSet, wallet modules.Wallet, tpool modules.TransactionPool, persistDir string, deps modules.Dependencies) (*Renter, error) {
	hdb, errChan := hostdb.NewCustomHostDB(g, cs, tpool, persistDir, deps)
	if err := <-errChan; err != nil {
		return nil, err
	}
	rl := ratelimit.NewRateLimit(0, 0, 0)
	contractSet, err := proto.NewContractSet(filepath.Join(persistDir, "contracts"), rl, modules.ProdDependencies)
	if err != nil {
		return nil, err
	}

	logger, err := persist.NewFileLogger(filepath.Join(persistDir, "contractor.log"))
	if err != nil {
		return nil, err
	}

	hc, errChan := contractor.NewCustomContractor(cs, wallet, tpool, hdb, persistDir, contractSet, logger, deps)
	if err := <-errChan; err != nil {
		return nil, err
	}
	renter, errChan := NewCustomRenter(g, cs, tpool, hdb, wallet, hc, persistDir, rl, deps)
	return renter, <-errChan
}

// // TestRenterCanAccessEphemeralAccountHostSettings verifies that the renter has
// // access to the host's external settings and that they include the new
// // ephemeral account setting fields.
// func TestRenterCanAccessEphemeralAccountHostSettings(t *testing.T) {
// 	if testing.Short() {
// 		t.SkipNow()
// 	}
// 	rt, err := newRenterTester(t.Name())
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	defer rt.Close()

// 	// Add a host to the test group
// 	h, err := rt.addHost(t.Name())
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	hostEntry, found, err := rt.renter.hostDB.Host(h.PublicKey())
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	if !found {
// 		t.Fatal("Expected the newly added host to be found in the hostDB")
// 	}

// 	if hostEntry.EphemeralAccountExpiry != modules.DefaultEphemeralAccountExpiry {
// 		t.Fatal("Unexpected account expiry")
// 	}

// 	if !hostEntry.MaxEphemeralAccountBalance.Equals(modules.DefaultMaxEphemeralAccountBalance) {
// 		t.Fatal("Unexpected max account balance")
// 	}
// }

// TestRenterPricesDivideByZero verifies that the Price Estimation catches
// divide by zero errors.
func TestRenterPricesDivideByZero(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rt, err := newRenterTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Confirm price estimation returns error if there are no hosts available
	_, _, err = rt.renter.PriceEstimation(modules.Allowance{})
	if err == nil {
		t.Fatal("Expected error due to no hosts")
	}

	// Add a host to the test group
	_, err = rt.addHost(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Confirm price estimation does not return an error now that there is a
	// host available
	_, _, err = rt.renter.PriceEstimation(modules.Allowance{})
	if err != nil {
		t.Fatal(err)
	}
}
