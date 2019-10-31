package consensus

import (
	"path/filepath"
	"testing"

	bolt "github.com/coreos/bbolt"
	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/SiaPrime/SiaPrime/build"
	"gitlab.com/SiaPrime/SiaPrime/crypto"
	"gitlab.com/SiaPrime/SiaPrime/encoding"
	"gitlab.com/SiaPrime/SiaPrime/modules"
	"gitlab.com/SiaPrime/SiaPrime/modules/gateway"
	"gitlab.com/SiaPrime/SiaPrime/modules/miner"
	"gitlab.com/SiaPrime/SiaPrime/modules/transactionpool"
	"gitlab.com/SiaPrime/SiaPrime/modules/wallet"
	"gitlab.com/SiaPrime/SiaPrime/types"
)

// A consensusSetTester is the helper object for consensus set testing,
// including helper modules and methods for controlling synchronization between
// the tester and the modules.
type consensusSetTester struct {
	gateway   modules.Gateway
	miner     modules.TestMiner
	tpool     modules.TransactionPool
	wallet    modules.Wallet
	walletKey crypto.CipherKey

	cs *ConsensusSet

	persistDir string
}

// randAddress returns a random address that is not spendable.
func randAddress() (uh types.UnlockHash) {
	fastrand.Read(uh[:])
	return
}

// addSiafunds makes a transaction that moves some testing genesis siafunds
// into the wallet.
func (cst *consensusSetTester) addSiafunds() {
	// Get an address to receive the siafunds.
	uc, err := cst.wallet.NextAddress()
	if err != nil {
		panic(err)
	}

	// Create the transaction that sends the anyone-can-spend siafund output to
	// the wallet address (output only available during testing).
	txn := types.Transaction{
		SiafundInputs: []types.SiafundInput{{
			ParentID:         cst.cs.blockRoot.Block.Transactions[1].SiafundOutputID(2),
			UnlockConditions: types.UnlockConditions{},
		}},
		SiafundOutputs: []types.SiafundOutput{{
			Value:      types.NewCurrency64(1e3),
			UnlockHash: uc.UnlockHash(),
		}},
	}

	// Mine the transaction into the blockchain.
	err = cst.tpool.AcceptTransactionSet([]types.Transaction{txn})
	if err != nil {
		panic(err)
	}
	_, err = cst.miner.AddBlock()
	if err != nil {
		panic(err)
	}

	// Check that the siafunds made it to the wallet.
	_, siafundBalance, _, err := cst.wallet.ConfirmedBalance()
	if err != nil {
		panic(err)
	}
	if !siafundBalance.Equals64(1e3) {
		panic("wallet does not have the siafunds")
	}
}

// mineCoins mines blocks until there are siacoins in the wallet.
func (cst *consensusSetTester) mineSiacoins() {
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		b, _ := cst.miner.FindBlock()
		err := cst.cs.AcceptBlock(b)
		if err != nil {
			panic(err)
		}
	}
}

// blankConsensusSetTester creates a consensusSetTester that has only the
// genesis block.
func blankConsensusSetTester(name string, deps modules.Dependencies) (*consensusSetTester, error) {
	testdir := build.TempDir(modules.ConsensusDir, name)

	// Create modules.
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, err := NewCustomConsensusSet(g, false, filepath.Join(testdir, modules.ConsensusDir), deps)
	if err != nil {
		return nil, err
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.ConsensusDir))
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

	// Assemble all objects into a consensusSetTester.
	cst := &consensusSetTester{
		gateway:   g,
		miner:     m,
		tpool:     tp,
		wallet:    w,
		walletKey: key,

		cs: cs,

		persistDir: testdir,
	}
	return cst, nil
}

// createConsensusSetTester creates a consensusSetTester that's ready for use,
// including siacoins and siafunds available in the wallet.
func createConsensusSetTester(name string) (*consensusSetTester, error) {
	cst, err := blankConsensusSetTester(name, modules.ProdDependencies)
	if err != nil {
		return nil, err
	}
	cst.addSiafunds()
	cst.mineSiacoins()
	return cst, nil
}

// Close safely closes the consensus set tester. Because there's not a good way
// to errcheck when deferring a close, a panic is called in the event of an
// error.
func (cst *consensusSetTester) Close() error {
	errs := []error{
		cst.cs.Close(),
		cst.gateway.Close(),
		cst.miner.Close(),
	}
	if err := build.JoinErrors(errs, "; "); err != nil {
		panic(err)
	}
	return nil
}

// TestNilInputs tries to create new consensus set modules using nil inputs.
func TestNilInputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	testdir := build.TempDir(modules.ConsensusDir, t.Name())
	_, err := New(nil, false, testdir)
	if err != errNilGateway {
		t.Fatal(err)
	}
}

// TestSiafundClaim calls SiafundClaim() function with different heights and
// siafund pool values set.
// Test checks how Siafund Emission Hardfork changes are handled when calculating
// claim balance by SiafundOutput on multiple edge cases.
func TestSiafundClaim(t *testing.T) {
	testdir := build.TempDir(modules.ConsensusDir, t.Name())

	// Create the gateway.
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := New(g, false, testdir)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		currentPool  types.Currency
		hardforkPool types.Currency
		height       types.BlockHeight
		sfo          types.SiafundOutput
		correctClaim types.Currency
	}{
		// Now is before hardfork.
		{types.NewCurrency64(1000000), types.ZeroCurrency, types.SpfHardforkHeight, types.SiafundOutput{Value: types.NewCurrency64(156), ClaimStart: types.ZeroCurrency}, types.NewCurrency64(15600)},
		// SFO with ClaimStart from before hardfork and now is after hardfork.
		{types.NewCurrency64(50000000), types.NewCurrency64(20000000), types.SpfHardforkHeight + 1, types.SiafundOutput{Value: types.NewCurrency64(1200), ClaimStart: types.NewCurrency64(10000000)}, types.NewCurrency64(2400000)},
		// New SFO with ClaimStart after hardfork and now is after hardfork.
		{types.NewCurrency64(80000000), types.NewCurrency64(20000000), types.SpfHardforkHeight * 100500, types.SiafundOutput{Value: types.NewCurrency64(20000), ClaimStart: types.NewCurrency64(50000000)}, types.NewCurrency64(20000000)},
	}

	for _, tc := range tests {
		err = cs.db.Update(func(tx *bolt.Tx) error {
			setSiafundPool(tx, tc.currentPool)
			setSiafundHardforkPool(tx, tc.hardforkPool)
			blockHeight := tx.Bucket(BlockHeight)
			return blockHeight.Put(BlockHeight, encoding.Marshal(tc.height))
		})
		if err != nil {
			t.Fatal(err)
		}
		claim := cs.SiafundClaim(tc.sfo)
		if !claim.Equals(tc.correctClaim) {
			cl, _ := claim.Float64()
			correct, _ := tc.correctClaim.Float64()
			t.Errorf("claim %v isn't equal to correct %v", cl, correct)
		}
	}

	err = cs.Close()
	if err != nil {
		t.Error(err)
	}
}

// TestClosing tries to close a consenuss set.
func TestDatabaseClosing(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	testdir := build.TempDir(modules.ConsensusDir, t.Name())

	// Create the gateway.
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := New(g, false, testdir)
	if err != nil {
		t.Fatal(err)
	}
	err = cs.Close()
	if err != nil {
		t.Error(err)
	}
}
