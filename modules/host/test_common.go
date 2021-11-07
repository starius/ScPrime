package host

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/siamux"
	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/consensus"
	"gitlab.com/scpcorp/ScPrime/modules/gateway"
	"gitlab.com/scpcorp/ScPrime/modules/miner"
	"gitlab.com/scpcorp/ScPrime/modules/transactionpool"
	"gitlab.com/scpcorp/ScPrime/modules/wallet"
	"gitlab.com/scpcorp/ScPrime/types"
)

// A hostTester is the helper object for host testing, including helper modules
// and methods for controlling synchronization.
type (
	closeFn func() error

	hostTester struct {
		mux *siamux.SiaMux

		cs        modules.ConsensusSet
		gateway   modules.Gateway
		miner     modules.TestMiner
		tpool     modules.TransactionPool
		wallet    modules.Wallet
		walletKey crypto.CipherKey

		host *Host

		persistDir string
	}

	// HostMock is almost full copy of the hostTester.
	HostMock struct {
		Mux *siamux.SiaMux

		CS        modules.ConsensusSet
		Gateway   modules.Gateway
		Miner     modules.TestMiner
		Tpool     modules.TransactionPool
		Wallet    modules.Wallet
		WalletKey crypto.CipherKey

		Host *Host
	}
)

// NewHostMock creates new public host mock based on private therefore do not affect old tests.
func NewHostMock(d modules.Dependencies, dirName string) (*HostMock, error) {
	h, err := newMockHostTester(d, dirName)
	if err != nil {
		return nil, err
	}

	h.host.mu.Lock()
	h.host.settings.AcceptingContracts = true
	h.host.mu.Unlock()

	renterWallet, err := createWallet(h.cs, h.tpool, h.miner, h.wallet, h.persistDir)
	if err != nil {
		return nil, fmt.Errorf("create renter wallet: %w", err)
	}

	return &HostMock{
		Mux:       h.mux,
		CS:        h.cs,
		Gateway:   h.gateway,
		Miner:     h.miner,
		Tpool:     h.tpool,
		Wallet:    renterWallet,
		WalletKey: h.walletKey,
		Host:      h.host,
	}, nil
}

func createWallet(cs modules.ConsensusSet, tp modules.TransactionPool, miner modules.TestMiner, hostWallet modules.Wallet, testdir string) (modules.Wallet, error) {
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir+"_2"))
	if err != nil {
		return nil, fmt.Errorf("new wallet: %w", err)
	}

	// Initialize the wallet and mine blocks until the wallet has money.
	// Create the keys for the wallet and unlock it.
	key := crypto.GenerateSiaKey(crypto.TypeDefaultWallet)
	_, err = w.Encrypt(key)
	if err != nil {
		return nil, fmt.Errorf("wallet encrypt: %w", err)
	}
	err = w.Unlock(key)
	if err != nil {
		return nil, fmt.Errorf("wallet unlock: %w", err)
	}

	renterAddr, err := w.NextAddress()
	if err != nil {
		return nil, fmt.Errorf("renter wallet next address: %w", err)
	}

	// Send money to renter's wallet.
	_, err = hostWallet.SendSiacoins(types.NewCurrency64(10000000000000000000).Mul(types.NewCurrency64(100000000)), renterAddr.UnlockHash())
	if err != nil {
		return nil, fmt.Errorf("host wallet send funds: %w", err)
	}

	// Add block to increase the wallet balance.
	_, err = miner.AddBlock()
	if err != nil {
		return nil, fmt.Errorf("add block: %w", err)
	}

	return w, nil
}

// blankMockHostTester creates a host tester where the modules are created but no
// extra initialization has been done, for example no blocks have been mined
// and the wallet keys have not been created.
func blankMockHostTester(d modules.Dependencies, name string) (*hostTester, error) {
	testdir := build.TempDir(modules.HostDir, name)

	// Create the siamux.
	siaMuxDir := filepath.Join(testdir, modules.SiaMuxDir)
	mux, err := modules.NewSiaMux(siaMuxDir, testdir, "localhost:0", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("new sia mux: %w", err)
	}

	// Create the modules.
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, fmt.Errorf("new gateway: %w", err)
	}
	cs, errChan := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err := <-errChan; err != nil {
		return nil, fmt.Errorf("new consensus: %w", err)
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		return nil, fmt.Errorf("new transaction pool: %w", err)
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, fmt.Errorf("new wallet: %w", err)
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, fmt.Errorf("new miner: %w", err)
	}

	h, err := NewCustomHost(d, cs, g, tp, w, mux, "localhost:0", filepath.Join(testdir, modules.HostDir), nil, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("new custom host: %w", err)
	}
	/*
		r, err := renter.New(cs, w, tp, filepath.Join(testdir, modules.RenterDir))
		if err != nil {
			return nil, err
		}
	*/

	// Assemble all objects into a hostTester
	ht := &hostTester{
		mux: mux,

		cs:      cs,
		gateway: g,
		miner:   m,
		// renter:  r,
		tpool:  tp,
		wallet: w,

		host: h,

		persistDir: testdir,
	}

	return ht, nil
}

// newMockHostTester creates a host tester with an initialized wallet and money
// in that wallet, using the dependencies provided.
func newMockHostTester(d modules.Dependencies, name string) (*hostTester, error) {
	// Create a blank host tester.
	ht, err := blankMockHostTester(d, name)
	if err != nil {
		return nil, fmt.Errorf("blanck mock host tester: %w", err)
	}

	// Initialize the wallet and mine blocks until the wallet has money.
	err = ht.initWallet()
	if err != nil {
		return nil, fmt.Errorf("init wallet: %w", err)
	}

	// TaxHardforkHeight == 10 for testing.
	for i := types.BlockHeight(0); i <= types.TaxHardforkHeight+1; i++ {
		_, err = ht.miner.AddBlock()
		if err != nil {
			return nil, fmt.Errorf("add block: %w", err)
		}
	}

	// Create two storage folder for the host, one the minimum size and one
	// twice the minimum size.
	storageFolderOne := filepath.Join(ht.persistDir, "hostTesterStorageFolderOne")
	err = os.Mkdir(storageFolderOne, 0700)
	if err != nil {
		return nil, fmt.Errorf("make first host dir: %w", err)
	}
	err = ht.host.AddStorageFolder(storageFolderOne, modules.SectorSize*64)
	if err != nil {
		return nil, fmt.Errorf("add first host dir: %w", err)
	}
	storageFolderTwo := filepath.Join(ht.persistDir, "hostTesterStorageFolderTwo")
	err = os.Mkdir(storageFolderTwo, 0700)
	if err != nil {
		return nil, fmt.Errorf("make second host dir: %w", err)
	}
	err = ht.host.AddStorageFolder(storageFolderTwo, modules.SectorSize*64*2)
	if err != nil {
		return nil, fmt.Errorf("add second host dir: %w", err)
	}

	//init the host.unlockHash to use in host transactions
	address, err := ht.wallet.NextAddress()
	if err != nil {
		return nil, errors.AddContext(err, "Unable to init host.Unlockhash")
	}
	ht.host.unlockHash = address.UnlockHash()
	return ht, nil
}

// initWallet creates a wallet key, initializes the host wallet, unlocks it,
// and then stores the key in the host tester.
func (ht *hostTester) initWallet() error {
	// Create the keys for the wallet and unlock it.
	key := crypto.GenerateSiaKey(crypto.TypeDefaultWallet)
	ht.walletKey = key
	_, err := ht.wallet.Encrypt(key)
	if err != nil {
		return fmt.Errorf("wallet encrypt: %w", err)
	}
	err = ht.wallet.Unlock(key)
	if err != nil {
		return fmt.Errorf("wallet unlock: %w", err)
	}
	return nil
}
