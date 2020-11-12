package consensus

import (
	"path/filepath"
	"testing"

	"gitlab.com/NebulousLabs/encoding"
	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/gateway"
	"gitlab.com/scpcorp/ScPrime/modules/miner"
	"gitlab.com/scpcorp/ScPrime/modules/transactionpool"
	"gitlab.com/scpcorp/ScPrime/modules/wallet"
	"gitlab.com/scpcorp/ScPrime/types"

	bolt "go.etcd.io/bbolt"
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
	cs, errChan := NewCustomConsensusSet(g, false, filepath.Join(testdir, modules.ConsensusDir), deps)
	if err := <-errChan; err != nil {
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
	_, errChan := New(nil, false, testdir)
	if err := <-errChan; err != errNilGateway {
		t.Fatal(err)
	}
}

// TestStoreSiafundHardforkPool checks database functions for getting and setting
// siafund hardfork pool values.
func TestStoreSiafundHardforkPool(t *testing.T) {
	testdir := build.TempDir(modules.ConsensusDir, t.Name())

	// Create the gateway + consensus.
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, errChan := New(g, false, testdir)
	if err := <-errChan; err != nil {
		t.Fatal(err)
	}
	val0 := types.NewCurrency64(138320348)
	val1 := types.NewCurrency64(46586868)
	err = cs.db.Update(func(tx *bolt.Tx) error {
		setSiafundHardforkPool(tx, val0, types.SpfHardforkHeight)
		setSiafundHardforkPool(tx, val1, types.SpfSecondHardforkHeight)
		pool0 := getSiafundHardforkPool(tx, types.SpfHardforkHeight)
		if pool0.Cmp(val0) != 0 {
			t.Errorf("retrieved pool value %v isn't equal to stored %v", pool0, val0)
		}
		pool1 := getSiafundHardforkPool(tx, types.SpfSecondHardforkHeight)
		if pool1.Cmp(val1) != 0 {
			t.Errorf("retrieved pool value %v isn't equal to stored %v", pool1, val1)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func testClaim(t *testing.T, claimStart, poolDiffPerBlock types.Currency, firstHf, secondHf hardforkInfo) {
	prevClaim := types.ZeroCurrency
	maxPool := poolDiffPerBlock.Mul64(uint64(types.SpfSecondHardforkHeight) + 100500)
	for currentPool := claimStart.Add(poolDiffPerBlock); currentPool.Cmp(maxPool) < 0; currentPool = currentPool.Add(poolDiffPerBlock) {
		claim := claimPerFund(claimStart, currentPool, firstHf, secondHf)
		if claim.Cmp(prevClaim) <= 0 {
			t.Errorf("claim per fund does not grow monotonically: claim: %v; prev: %v; claimStart: %v", claim, prevClaim, claimStart)
		}
		if currentPool.Cmp(firstHf.pool) == 0 {
			firstHf.isActivated = true
		}
		if currentPool.Cmp(secondHf.pool) == 0 {
			secondHf.isActivated = true
		}
		prevClaim = claim
	}
}

// TestClaimPerFundMonotonicGrowth checks that total claim of one fund
// increases monotonically during all periods of blockchain history.
func TestClaimPerFundMonotonicGrowth(t *testing.T) {
	poolDiffPerBlock := types.NewCurrency64(200000000)
	firstHf := hardforkInfo{pool: poolDiffPerBlock.Mul64(uint64(types.SpfHardforkHeight))}
	secondHf := hardforkInfo{pool: poolDiffPerBlock.Mul64(uint64(types.SpfSecondHardforkHeight))}
	// Test different claim starts.
	tests := []types.Currency{
		types.ZeroCurrency,
		poolDiffPerBlock,
		firstHf.pool.Sub(poolDiffPerBlock),
		firstHf.pool,
		firstHf.pool.Add(poolDiffPerBlock),
		secondHf.pool.Sub(poolDiffPerBlock),
		secondHf.pool,
		secondHf.pool.Add(poolDiffPerBlock),
	}
	for _, claimStart := range tests {
		testClaim(t, claimStart, poolDiffPerBlock, firstHf, secondHf)
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
	cs, errChan := New(g, false, testdir)
	if err := <-errChan; err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		currentPool   types.Currency
		hardforkPool0 types.Currency
		hardforkPool1 types.Currency
		height        types.BlockHeight
		sfo           types.SiafundOutput
		correctClaim  types.Currency
		description   string
	}{
		{types.NewCurrency64(1000000), types.ZeroCurrency, types.ZeroCurrency, types.SpfHardforkHeight, types.SiafundOutput{Value: types.NewCurrency64(156), ClaimStart: types.ZeroCurrency}, types.NewCurrency64(15600), "Test before the first hardfork"},
		{types.NewCurrency64(50000000), types.NewCurrency64(20000000), types.ZeroCurrency, types.SpfHardforkHeight + 1, types.SiafundOutput{Value: types.NewCurrency64(1200), ClaimStart: types.NewCurrency64(10000000)}, types.NewCurrency64(2400000), "Test SFO with ClaimStart from before the first hardfork and now is after the first hardfork"},
		{types.NewCurrency64(80000000), types.NewCurrency64(20000000), types.ZeroCurrency, types.SpfSecondHardforkHeight, types.SiafundOutput{Value: types.NewCurrency64(20000), ClaimStart: types.NewCurrency64(50000000)}, types.NewCurrency64(20000000), "Test new SFO with ClaimStart after the first hardfork and now is after the first hardfork"},
		{types.NewCurrency64(800000000), types.NewCurrency64(200000000), types.NewCurrency64(400000000), types.SpfSecondHardforkHeight + 1, types.SiafundOutput{Value: types.NewCurrency64(100000000), ClaimStart: types.NewCurrency64(600000000)}, types.NewCurrency64(100000000), "Test new SFO with ClaimStart after the second hardfork and now is after the second hardfork"},
		{types.NewCurrency64(800000000), types.NewCurrency64(100000000), types.NewCurrency64(300000000), types.SpfSecondHardforkHeight * 10000, types.SiafundOutput{Value: types.NewCurrency64(15000), ClaimStart: types.NewCurrency64(200000000)}, types.NewCurrency64(50025000), "Test new SFO with ClaimStart between hardforks and now is after the second hardfork"},
		{types.NewCurrency64(800000000), types.NewCurrency64(100000000), types.NewCurrency64(300000000), types.SpfSecondHardforkHeight * 10000, types.SiafundOutput{Value: types.NewCurrency64(10000), ClaimStart: types.NewCurrency64(0)}, types.NewCurrency64(166680000), "Test new SFO with ClaimStart before the first hardfork and now is after the second hardfork"},
	}

	for _, tc := range tests {
		err = cs.db.Update(func(tx *bolt.Tx) error {
			setSiafundPool(tx, tc.currentPool)
			if tc.height >= types.SpfHardforkHeight {
				setSiafundHardforkPool(tx, tc.hardforkPool0, types.SpfHardforkHeight)
			}
			if tc.height >= types.SpfSecondHardforkHeight {
				setSiafundHardforkPool(tx, tc.hardforkPool1, types.SpfSecondHardforkHeight)
			}
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
			t.Errorf("claim %v isn't equal to correct %v; test: %s", cl, correct, tc.description)
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
	cs, errChan := New(g, false, testdir)
	if err := <-errChan; err != nil {
		t.Fatal(err)
	}
	err = cs.Close()
	if err != nil {
		t.Error(err)
	}
}
