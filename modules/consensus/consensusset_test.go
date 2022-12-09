package consensus

import (
	"os"
	"path/filepath"
	"testing"

	fuzz "github.com/google/gofuzz"
	"github.com/starius/unifynil"
	"github.com/stretchr/testify/require"
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
	t.Cleanup(func() {
		os.RemoveAll(testdir)
	})
	_, errChan := New(nil, false, testdir)
	if err := <-errChan; err != errNilGateway {
		t.Fatal(err)
	}
}

// setSiafundHardforkPoolLegacy sets the siafund hardfork pool value using legacy format.
// Legacy format is one constant key for hf pool value.
func setSiafundHardforkPoolLegacy(tx *bolt.Tx, c types.Currency) {
	bucket := tx.Bucket(SiafundHardforkPool)
	if bucket == nil {
		bucket, _ = tx.CreateBucket(SiafundHardforkPool)
	}
	err := bucket.Put(SiafundHardforkPool, encoding.Marshal(c))
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// TestStoreSiafundHardforkPool checks database functions for getting and setting
// siafund hardfork pool values.
func TestStoreSiafundHardforkPool(t *testing.T) {
	testdir := build.TempDir(modules.ConsensusDir, t.Name())
	t.Cleanup(func() {
		os.RemoveAll(testdir)
	})

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

	// Test old format compatibility.
	err = cs.db.Update(func(tx *bolt.Tx) error {
		setSiafundHardforkPoolLegacy(tx, val0)
		pool0 := getSiafundHardforkPool(tx, types.SpfHardforkHeight)
		if pool0.Cmp(val0) != 0 {
			t.Errorf("retrieved pool value %v isn't equal to stored %v", pool0, val0)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Test new format (height > hf pool).
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
		claim := claimPerFund(claimStart, currentPool, []hardforkInfo{firstHf, secondHf}, []claimRange{{start: claimStart, end: currentPool}})
		if claim.Total.Cmp(prevClaim) <= 0 {
			t.Errorf("claim per fund does not grow monotonically: claim: %v; prev: %v; claimStart: %v", claim, prevClaim, claimStart)
		}
		if currentPool.Cmp(firstHf.pool) == 0 {
			firstHf.isActivated = true
		}
		if currentPool.Cmp(secondHf.pool) == 0 {
			secondHf.isActivated = true
		}
		prevClaim = claim.Total
	}
}

// TestClaimPerFundMonotonicGrowth checks that total claim of one fund
// increases monotonically during all periods of blockchain history.
func TestClaimPerFundMonotonicGrowth(t *testing.T) {
	poolDiffPerBlock := types.NewCurrency64(200000000)
	firstHf := hardforkInfo{pool: poolDiffPerBlock.Mul64(uint64(types.SpfHardforkHeight)), siafundCount: types.NewSiafundCount}
	secondHf := hardforkInfo{pool: poolDiffPerBlock.Mul64(uint64(types.SpfSecondHardforkHeight)), siafundCount: types.NewerSiafundCount}
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
	t.Cleanup(func() {
		os.RemoveAll(testdir)
	})

	// Create the gateway.
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, errChan := New(g, false, testdir)
	if err := <-errChan; err != nil {
		t.Fatal(err)
	}

	type claimsWithRange struct {
		FileContractRange
		startClaim types.Currency
		endClaim   types.Currency
	}

	tests := []struct {
		currentPool    types.Currency
		hardforkPools  map[types.BlockHeight]types.Currency
		height         types.BlockHeight
		contractRanges []claimsWithRange
		spfB           bool
		sfoid          types.SiafundOutputID
		sfo            types.SiafundOutput
		correctClaim   types.SiafundClaim
		description    string
	}{
		{
			types.NewCurrency64(1000000),
			map[types.BlockHeight]types.Currency{
				types.SpfHardforkHeight:       types.ZeroCurrency,
				types.SpfSecondHardforkHeight: types.ZeroCurrency,
			},
			types.SpfHardforkHeight,
			[]claimsWithRange{},
			false,
			types.SiafundOutputID(crypto.Hash{1}),
			types.SiafundOutput{Value: types.NewCurrency64(156), ClaimStart: types.ZeroCurrency, UnlockHash: types.UnlockHash(crypto.Hash{111})},
			types.SiafundClaim{Total: types.NewCurrency64(15600), ByOwner: types.NewCurrency64(15600)},
			"Test before the first hardfork",
		},
		{
			types.NewCurrency64(50000000),
			map[types.BlockHeight]types.Currency{
				types.SpfHardforkHeight:       types.NewCurrency64(20000000),
				types.SpfSecondHardforkHeight: types.ZeroCurrency,
			},
			types.SpfHardforkHeight + 1,
			[]claimsWithRange{},
			false,
			types.SiafundOutputID(crypto.Hash{2}),
			types.SiafundOutput{Value: types.NewCurrency64(1200), ClaimStart: types.NewCurrency64(10000000), UnlockHash: types.UnlockHash(crypto.Hash{112})},
			types.SiafundClaim{Total: types.NewCurrency64(2400000), ByOwner: types.NewCurrency64(2400000)},
			"Test SFO with ClaimStart from before the first hardfork and now is after the first hardfork",
		},
		{
			types.NewCurrency64(80000000),
			map[types.BlockHeight]types.Currency{
				types.SpfHardforkHeight:       types.NewCurrency64(20000000),
				types.SpfSecondHardforkHeight: types.ZeroCurrency,
			},
			types.SpfSecondHardforkHeight,
			[]claimsWithRange{},
			false,
			types.SiafundOutputID(crypto.Hash{3}),
			types.SiafundOutput{Value: types.NewCurrency64(20000), ClaimStart: types.NewCurrency64(50000000), UnlockHash: types.UnlockHash(crypto.Hash{113})},
			types.SiafundClaim{Total: types.NewCurrency64(20000000), ByOwner: types.NewCurrency64(20000000)},
			"Test new SFO with ClaimStart after the first hardfork and now is after the first hardfork",
		},
		{
			types.NewCurrency64(800000000),
			map[types.BlockHeight]types.Currency{
				types.SpfHardforkHeight:       types.NewCurrency64(200000000),
				types.SpfSecondHardforkHeight: types.NewCurrency64(400000000),
			},
			types.SpfSecondHardforkHeight + 1,
			[]claimsWithRange{},
			false,
			types.SiafundOutputID(crypto.Hash{4}),
			types.SiafundOutput{Value: types.NewCurrency64(100000000), ClaimStart: types.NewCurrency64(600000000), UnlockHash: types.UnlockHash(crypto.Hash{114})},
			types.SiafundClaim{Total: types.NewCurrency64(100000000), ByOwner: types.NewCurrency64(100000000)},
			"Test new SFO with ClaimStart after the second hardfork and now is after the second hardfork",
		},
		{
			types.NewCurrency64(800000000),
			map[types.BlockHeight]types.Currency{
				types.SpfHardforkHeight:       types.NewCurrency64(100000000),
				types.SpfSecondHardforkHeight: types.NewCurrency64(300000000),
			},
			types.SpfSecondHardforkHeight + 1,
			[]claimsWithRange{},
			false,
			types.SiafundOutputID(crypto.Hash{5}),
			types.SiafundOutput{Value: types.NewCurrency64(15000), ClaimStart: types.NewCurrency64(200000000), UnlockHash: types.UnlockHash(crypto.Hash{115})},
			types.SiafundClaim{Total: types.NewCurrency64(50025000), ByOwner: types.NewCurrency64(50025000)},
			"Test new SFO with ClaimStart between hardforks and now is after the second hardfork",
		},
		{
			types.NewCurrency64(800000000),
			map[types.BlockHeight]types.Currency{
				types.SpfHardforkHeight:       types.NewCurrency64(100000000),
				types.SpfSecondHardforkHeight: types.NewCurrency64(300000000),
			},
			types.SpfSecondHardforkHeight + 1,
			[]claimsWithRange{},
			false,
			types.SiafundOutputID(crypto.Hash{6}),
			types.SiafundOutput{Value: types.NewCurrency64(10000), ClaimStart: types.NewCurrency64(0), UnlockHash: types.UnlockHash(crypto.Hash{116})},
			types.SiafundClaim{Total: types.NewCurrency64(166680000), ByOwner: types.NewCurrency64(166680000)},
			"Test new SFO with ClaimStart before the first hardfork and now is after the second hardfork",
		},
		{
			types.NewCurrency64(1200000000),
			map[types.BlockHeight]types.Currency{
				types.SpfHardforkHeight:       types.NewCurrency64(100000000),
				types.SpfSecondHardforkHeight: types.NewCurrency64(400000000),
				types.Fork2022Height:          types.NewCurrency64(800000000),
			},
			types.Fork2022Height + 1,
			[]claimsWithRange{},
			false,
			types.SiafundOutputID(crypto.Hash{7}),
			types.SiafundOutput{Value: types.NewCurrency64(10000), ClaimStart: types.NewCurrency64(0), UnlockHash: types.UnlockHash(crypto.Hash{117})},
			types.SiafundClaim{Total: types.NewCurrency64(200030000), ByOwner: types.NewCurrency64(200030000)},
			"Test SPF-A with ClaimStart before the first hardfork and now is after the Fork2022",
		},
		{
			types.NewCurrency64(2400000000),
			map[types.BlockHeight]types.Currency{
				types.SpfHardforkHeight:       types.NewCurrency64(100000000),
				types.SpfSecondHardforkHeight: types.NewCurrency64(400000000),
				types.Fork2022Height:          types.NewCurrency64(800000000),
			},
			types.Fork2022Height + 100000,
			[]claimsWithRange{
				{FileContractRange{Start: types.Fork2022Height + 14869, End: types.Fork2022Height + 18869}, types.NewCurrency64(800000000), types.NewCurrency64(1600000000)},
				{FileContractRange{Start: types.Fork2022Height + 15869, End: types.Fork2022Height + 19869}, types.NewCurrency64(1200000000), types.NewCurrency64(2000000000)},
				{FileContractRange: FileContractRange{Start: types.Fork2022Height + 100000, End: types.Fork2022Height + 104000}, startClaim: types.NewCurrency64(2400000000)},
			},
			true,
			types.SiafundOutputID(crypto.Hash{8}),
			types.SiafundOutput{Value: types.NewCurrency64(25000), ClaimStart: types.NewCurrency64(800000000), UnlockHash: types.UnlockHash(crypto.Hash{118})},
			types.SiafundClaim{Total: types.NewCurrency64(100000), ByOwner: types.NewCurrency64(75000)},
			"Test SPF-B with ClaimStart after the Fork2022 and now is after the Fork2022",
		},
		{
			types.NewCurrency64(2800000000),
			map[types.BlockHeight]types.Currency{
				types.SpfHardforkHeight:       types.NewCurrency64(100000000),
				types.SpfSecondHardforkHeight: types.NewCurrency64(400000000),
				types.Fork2022Height:          types.NewCurrency64(800000000),
			},
			types.Fork2022Height + 100000,
			[]claimsWithRange{
				{FileContractRange{Start: types.Fork2022Height + 14869, End: types.Fork2022Height + 18869}, types.NewCurrency64(800000000), types.NewCurrency64(1600000000)},
				{FileContractRange{Start: types.Fork2022Height + 15869, End: types.Fork2022Height + 19869}, types.NewCurrency64(1200000000), types.NewCurrency64(2000000000)},
				{FileContractRange: FileContractRange{Start: types.Fork2022Height + 98000, End: types.Fork2022Height + 102000}, startClaim: types.NewCurrency64(2400000000)},
			},
			true,
			types.SiafundOutputID(crypto.Hash{9}),
			types.SiafundOutput{Value: types.NewCurrency64(25000), ClaimStart: types.NewCurrency64(800000000), UnlockHash: types.UnlockHash(crypto.Hash{119})},
			types.SiafundClaim{Total: types.NewCurrency64(125000), ByOwner: types.NewCurrency64(100000)},
			"Test SPF-B with ClaimStart after the Fork2022 and now is after the Fork2022",
		},
		{
			types.NewCurrency64(2800000000),
			map[types.BlockHeight]types.Currency{
				types.SpfHardforkHeight:       types.NewCurrency64(100000000),
				types.SpfSecondHardforkHeight: types.NewCurrency64(400000000),
				types.Fork2022Height:          types.NewCurrency64(1200000000),
			},
			types.Fork2022Height + 100000,
			[]claimsWithRange{
				{FileContractRange{Start: types.SpfSecondHardforkHeight - 2000, End: types.SpfSecondHardforkHeight + 2000}, types.NewCurrency64(100000000), types.NewCurrency64(600000000)},
				{FileContractRange{Start: types.Fork2022Height - 1000, End: types.Fork2022Height + 3000}, types.NewCurrency64(800000000), types.NewCurrency64(1600000000)},
			},
			true,
			types.SiafundOutputID(crypto.Hash{10}),
			types.SiafundOutput{Value: types.NewCurrency64(20000), ClaimStart: types.NewCurrency64(100000000), UnlockHash: types.UnlockHash(crypto.Hash{120})},
			types.SiafundClaim{Total: types.NewCurrency64(200160000), ByOwner: types.NewCurrency64(200080000)},
			"Test SPF-B with contracts covering hardfork points",
		},
		{
			types.NewCurrency64(2800000000),
			map[types.BlockHeight]types.Currency{
				types.SpfHardforkHeight:       types.NewCurrency64(100000000),
				types.SpfSecondHardforkHeight: types.NewCurrency64(400000000),
				types.Fork2022Height:          types.NewCurrency64(800000000),
			},
			types.Fork2022Height + 100000,
			[]claimsWithRange{},
			true,
			types.SiafundOutputID(crypto.Hash{11}),
			types.SiafundOutput{Value: types.NewCurrency64(25000), ClaimStart: types.NewCurrency64(800000000), UnlockHash: types.UnlockHash(crypto.Hash{130})},
			types.SiafundClaim{Total: types.NewCurrency64(125000), ByOwner: types.NewCurrency64(0)},
			"Test SPF-B without contracts; after Fork2022",
		},
	}

	for _, tc := range tests {
		err = cs.db.Update(func(tx *bolt.Tx) error {
			for _, state := range types.SiafundStates {
				if tc.height >= state.At {
					setSiafundHardforkPool(tx, tc.hardforkPools[state.At], state.At)
				}
			}
			setSiafundPool(tx, tc.currentPool)
			addSiafundOutput(tx, tc.sfoid, tc.sfo)
			if tc.spfB {
				addSiafundBOutput(tx, tc.sfoid)
				for _, cr := range tc.contractRanges {
					addFileContractRange(tx, []types.UnlockHash{tc.sfo.UnlockHash}, cr.FileContractRange)
					setSiafundHistoricalPool(tx, cr.startClaim, cr.Start)
					if tc.height >= cr.End {
						setSiafundHistoricalPool(tx, cr.endClaim, cr.End)
					}
				}
			}
			blockHeight := tx.Bucket(BlockHeight)
			return blockHeight.Put(BlockHeight, encoding.Marshal(tc.height))
		})
		if err != nil {
			t.Fatal(err)
		}
		claim, err := cs.SiafundClaim(tc.sfoid)
		if err != nil {
			t.Fatal(err)
		}
		if !claim.Total.Equals(tc.correctClaim.Total) {
			cl, _ := claim.Total.Float64()
			correct, _ := tc.correctClaim.Total.Float64()
			t.Errorf("claim %v isn't equal to correct %v; test: %s", cl, correct, tc.description)
		}
		if !claim.ByOwner.Equals(tc.correctClaim.ByOwner) {
			cl, _ := claim.ByOwner.Float64()
			correct, _ := tc.correctClaim.ByOwner.Float64()
			t.Errorf("claim %v isn't equal to correct %v; test: %s", cl, correct, tc.description)
		}
	}

	err = cs.Close()
	if err != nil {
		t.Error(err)
	}
}

// TestNonOverlappingRanges tests nonOverlappingRanges() function.
func TestNonOverlappingRanges(t *testing.T) {
	tests := []struct {
		ranges []FileContractRange
		result []FileContractRange
	}{
		{ranges: []FileContractRange{{types.BlockHeight(1), types.BlockHeight(101)}, {types.BlockHeight(50), types.BlockHeight(101)}, {types.BlockHeight(150), types.BlockHeight(250)}}, result: []FileContractRange{{types.BlockHeight(1), types.BlockHeight(101)}, {types.BlockHeight(150), types.BlockHeight(250)}}},
		{ranges: []FileContractRange{{types.BlockHeight(1), types.BlockHeight(101)}, {types.BlockHeight(50), types.BlockHeight(151)}, {types.BlockHeight(150), types.BlockHeight(250)}}, result: []FileContractRange{{types.BlockHeight(1), types.BlockHeight(250)}}},
		{ranges: []FileContractRange{{types.BlockHeight(1000), types.BlockHeight(6000)}, {types.BlockHeight(200500), types.BlockHeight(204500)}, {types.BlockHeight(201000), types.BlockHeight(202000)}, {types.BlockHeight(1), types.BlockHeight(5000)}}, result: []FileContractRange{{types.BlockHeight(1), types.BlockHeight(6000)}, {types.BlockHeight(200500), types.BlockHeight(204500)}}},
		{ranges: []FileContractRange{{types.BlockHeight(1000), types.BlockHeight(6000)}, {types.BlockHeight(200500), types.BlockHeight(204500)}, {types.BlockHeight(204500), types.BlockHeight(208000)}, {types.BlockHeight(6000), types.BlockHeight(7000)}}, result: []FileContractRange{{types.BlockHeight(1000), types.BlockHeight(7000)}, {types.BlockHeight(200500), types.BlockHeight(208000)}}},
	}
	for _, tc := range tests {
		require.Equal(t, tc.result, nonOverlappingRanges(tc.ranges))
	}
}

// TestCutClaimRanges checks cutClaimRanges() function.
func TestCutClaimRanges(t *testing.T) {
	tests := []struct {
		ranges []claimRange
		start  types.Currency
		end    types.Currency
		result []claimRange
	}{
		{ranges: []claimRange{{start: types.NewCurrency64(100), end: types.NewCurrency64(200)}}, start: types.NewCurrency64(0), end: types.NewCurrency64(300), result: []claimRange{{start: types.NewCurrency64(100), end: types.NewCurrency64(200)}}},
		{ranges: []claimRange{{start: types.NewCurrency64(10), end: types.NewCurrency64(20)}, {start: types.NewCurrency64(100), end: types.NewCurrency64(200)}}, start: types.NewCurrency64(0), end: types.NewCurrency64(300), result: []claimRange{{start: types.NewCurrency64(10), end: types.NewCurrency64(20)}, {start: types.NewCurrency64(100), end: types.NewCurrency64(200)}}},
		{ranges: []claimRange{{start: types.NewCurrency64(100), end: types.NewCurrency64(200)}}, start: types.NewCurrency64(0), end: types.NewCurrency64(50), result: nil},
		{ranges: []claimRange{{start: types.NewCurrency64(100), end: types.NewCurrency64(200)}, {start: types.NewCurrency64(100000), end: types.NewCurrency64(200000)}}, start: types.NewCurrency64(0), end: types.NewCurrency64(50), result: nil},
		{ranges: []claimRange{{start: types.NewCurrency64(100), end: types.NewCurrency64(200)}}, start: types.NewCurrency64(300), end: types.NewCurrency64(450), result: nil},
		{ranges: []claimRange{{start: types.NewCurrency64(0), end: types.NewCurrency64(100)}, {start: types.NewCurrency64(100), end: types.NewCurrency64(200)}}, start: types.NewCurrency64(300), end: types.NewCurrency64(450), result: nil},
		{ranges: []claimRange{{start: types.NewCurrency64(10), end: types.NewCurrency64(20)}, {start: types.NewCurrency64(100), end: types.NewCurrency64(200)}, {start: types.NewCurrency64(200), end: types.NewCurrency64(8080)}}, start: types.NewCurrency64(0), end: types.NewCurrency64(300), result: []claimRange{{start: types.NewCurrency64(10), end: types.NewCurrency64(20)}, {start: types.NewCurrency64(100), end: types.NewCurrency64(200)}, {start: types.NewCurrency64(200), end: types.NewCurrency64(300)}}},
		{ranges: []claimRange{{start: types.NewCurrency64(10), end: types.NewCurrency64(20)}, {start: types.NewCurrency64(100), end: types.NewCurrency64(200)}, {start: types.NewCurrency64(200), end: types.NewCurrency64(8080)}}, start: types.NewCurrency64(0), end: types.NewCurrency64(150), result: []claimRange{{start: types.NewCurrency64(10), end: types.NewCurrency64(20)}, {start: types.NewCurrency64(100), end: types.NewCurrency64(150)}}},
		{ranges: []claimRange{{start: types.NewCurrency64(56700), end: types.NewCurrency64(60700)}, {start: types.NewCurrency64(60700), end: types.NewCurrency64(64700)}, {start: types.NewCurrency64(101543), end: types.NewCurrency64(109000)}}, start: types.NewCurrency64(60001), end: types.NewCurrency64(103004), result: []claimRange{{start: types.NewCurrency64(60001), end: types.NewCurrency64(60700)}, {start: types.NewCurrency64(60700), end: types.NewCurrency64(64700)}, {start: types.NewCurrency64(101543), end: types.NewCurrency64(103004)}}},
		{ranges: []claimRange{{start: types.NewCurrency64(100), end: types.NewCurrency64(100)}}, start: types.NewCurrency64(100), end: types.NewCurrency64(300), result: []claimRange{{start: types.NewCurrency64(100), end: types.NewCurrency64(100)}}},
		{ranges: []claimRange{{start: types.NewCurrency64(100), end: types.NewCurrency64(100)}}, start: types.NewCurrency64(0), end: types.NewCurrency64(100), result: []claimRange{{start: types.NewCurrency64(100), end: types.NewCurrency64(100)}}},
		{ranges: []claimRange{{start: types.NewCurrency64(100), end: types.NewCurrency64(101)}}, start: types.NewCurrency64(0), end: types.NewCurrency64(100), result: []claimRange{{start: types.NewCurrency64(100), end: types.NewCurrency64(100)}}},
	}
	for _, tc := range tests {
		require.Equal(t, tc.result, cutClaimRanges(tc.ranges, tc.start, tc.end))
	}
}

// TestFileContractRangeMarshalUnmarshal checks FileContractRange serialisation and parsing.
func TestFileContractRangeMarshalUnmarshal(t *testing.T) {
	fcrSmallHeight := &FileContractRange{Start: 100, End: 4100}
	data := fcrSmallHeight.Marshal()
	// Make sure encoding uses varints.
	require.Equal(t, 3, len(data))
	fcrSmallHeight2 := &FileContractRange{}
	bytesRead := fcrSmallHeight2.Unmarshal(data)
	require.Equal(t, fcrSmallHeight, fcrSmallHeight2)
	require.Equal(t, len(data), bytesRead)

	f := fuzz.New()
	for i := 0; i < 100; i++ {
		fcr := &FileContractRange{}
		f.Fuzz(fcr)
		data := fcr.Marshal()
		fcr2 := &FileContractRange{}
		bytesRead = fcr2.Unmarshal(data)
		require.Equal(t, fcr, fcr2)
		require.Equal(t, len(data), bytesRead)
	}
}

// TestFileContractOwnerDiffs tests applying and reverting of FileContractOwnerDiff.
func TestFileContractOwnerDiffs(t *testing.T) {
	testdir := build.TempDir(modules.ConsensusDir, t.Name())
	t.Cleanup(func() {
		os.RemoveAll(testdir)
	})

	// Create the gateway + consensus.
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, errChan := New(g, false, testdir)
	if err := <-errChan; err != nil {
		t.Fatal(err)
	}

	// Generate data.
	var owners []types.UnlockHash
	var heights []types.BlockHeight
	var contractIDs []types.FileContractID
	f := fuzz.New().NumElements(10, 100).NilChance(0)
	f.Fuzz(&owners)
	f.Fuzz(&heights)
	f.Fuzz(&contractIDs)

	type contractOwnerState struct {
		ownerships map[types.FileContractID]*FileContractOwnership
		ranges     map[types.UnlockHash][]FileContractRange
	}
	tests := []struct {
		diff   modules.FileContractOwnerDiff
		before contractOwnerState
		after  contractOwnerState
	}{
		{
			diff: modules.FileContractOwnerDiff{
				Direction:   modules.DiffApply,
				ID:          contractIDs[0],
				Owners:      []types.UnlockHash{owners[0], owners[1]},
				StartHeight: heights[0],
				EndHeight:   heights[0] + heights[1],
			},
			before: contractOwnerState{
				ownerships: map[types.FileContractID]*FileContractOwnership{
					contractIDs[0]: nil,
					contractIDs[1]: {Start: heights[2], Owners: []types.UnlockHash{owners[0]}},
				},
				ranges: map[types.UnlockHash][]FileContractRange{
					owners[0]: {{Start: heights[2], End: heights[2] + heights[3]}},
					owners[1]: {},
				},
			},
			after: contractOwnerState{
				ownerships: map[types.FileContractID]*FileContractOwnership{
					contractIDs[0]: {Start: heights[0], Owners: []types.UnlockHash{owners[0], owners[1]}},
					contractIDs[1]: {Start: heights[2], Owners: []types.UnlockHash{owners[0]}},
				},
				ranges: map[types.UnlockHash][]FileContractRange{
					owners[0]: {{Start: heights[2], End: heights[2] + heights[3]}, {Start: heights[0], End: heights[0] + heights[1]}},
					owners[1]: {{Start: heights[0], End: heights[0] + heights[1]}},
				},
			},
		},
	}

	err = cs.db.Update(func(tx *bolt.Tx) error {
		checkState := func(expected contractOwnerState) {
			for contractID, ownership := range expected.ownerships {
				gotOwnership, err := getFileContractOwnership(tx, contractID)
				if ownership == nil {
					require.Equal(t, errNilItem, err)
				} else {
					require.NoError(t, err)
					require.Equal(t, *ownership, gotOwnership)
				}
			}
			for owner, ranges := range expected.ranges {
				gotRanges := getFileContractRanges(tx, owner)
				unifynil.Unify(&gotRanges, unifynil.SliceToEmpty())
				unifynil.Unify(&ranges, unifynil.SliceToEmpty())
				require.Equal(t, ranges, gotRanges)
			}
		}

		for _, tc := range tests {
			// Set `before` state.
			for owner, ranges := range tc.before.ranges {
				for _, r := range ranges {
					if len(ranges) != 0 {
						addFileContractRange(tx, []types.UnlockHash{owner}, r)
					}
				}
			}
			for contractID, ownership := range tc.before.ownerships {
				if ownership != nil {
					addFileContractOwnership(tx, contractID, *ownership)
				}
			}
			// Apply diff.
			commitFileContractOwnerDiff(tx, tc.diff, modules.DiffApply)
			// Check state after applying the diff.
			checkState(tc.after)
			// Revert the diff and check that state is reverted.
			commitFileContractOwnerDiff(tx, tc.diff, modules.DiffRevert)
			checkState(tc.before)
			// Cleanup `before` state.
			for owner, ranges := range tc.before.ranges {
				for _, r := range ranges {
					removeFileContractRange(tx, []types.UnlockHash{owner}, r)
				}
			}
			for contractID := range tc.before.ownerships {
				removeFileContractOwnership(tx, contractID)
			}
		}
		return nil
	})
	require.NoError(t, err)
}

// TestRemoveFileContractRange checks removeFileContractRange function.
func TestRemoveFileContractRange(t *testing.T) {
	testdir := build.TempDir(modules.ConsensusDir, t.Name())
	t.Cleanup(func() {
		os.RemoveAll(testdir)
	})

	// Create the gateway.
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, errChan := New(g, false, testdir)
	if err := <-errChan; err != nil {
		t.Fatal(err)
	}

	var owner types.UnlockHash
	var ranges []FileContractRange
	f := fuzz.New().NumElements(100, 1000).NilChance(0)
	f.Fuzz(&owner)
	f.Fuzz(&ranges)

	err = cs.db.Update(func(tx *bolt.Tx) error {
		for _, r := range ranges {
			addFileContractRange(tx, []types.UnlockHash{owner}, r)
		}
		for len(ranges) != 0 {
			removeIndex := fastrand.Intn(len(ranges))
			removeFileContractRange(tx, []types.UnlockHash{owner}, ranges[removeIndex])
			ranges = append(ranges[:removeIndex], ranges[removeIndex+1:]...)
			gotRanges := getFileContractRanges(tx, owner)
			unifynil.Unify(&gotRanges, unifynil.SliceToEmpty())
			require.Equal(t, ranges, gotRanges)
		}
		return nil
	})
	require.NoError(t, err)
}

// TestClosing tries to close a consenuss set.
func TestDatabaseClosing(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	testdir := build.TempDir(modules.ConsensusDir, t.Name())
	t.Cleanup(func() {
		os.RemoveAll(testdir)
	})

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

func TestDecodingNewFields(t *testing.T) {
	type Example struct {
		A, B int
	}
	type Example2 struct {
		Example
		NewField []int
	}
	type Example3 struct {
		NewField []int
		Example
	}
	data := encoding.Marshal(Example{A: 100, B: 200})
	ex2 := &Example2{}
	require.Error(t, encoding.Unmarshal(data, ex2))
	t.Logf("%+v", ex2)
	ex3 := &Example3{}
	require.Error(t, encoding.Unmarshal(data, ex3))
	t.Logf("%+v", ex3)
}
