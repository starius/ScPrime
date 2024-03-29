package consensus

// consensusdb.go contains all of the functions related to performing consensus
// related actions on the database, including initializing the consensus
// portions of the database. Many errors cause panics instead of being handled
// gracefully, but only when the debug flag is set. The errors are silently
// ignored otherwise, which is suboptimal.

import (
	"encoding/binary"
	"gitlab.com/NebulousLabs/encoding"
	bolt "go.etcd.io/bbolt"
	"sort"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
)

var (
	prefixDSCO = []byte("dsco_")
	prefixFCEX = []byte("fcex_")
)

var (
	// BlockHeight is a bucket that stores the current block height.
	//
	// Generally we would just look at BlockPath.Stats(), but there is an error
	// in boltdb that prevents the bucket stats from updating until a tx is
	// committed. Wasn't a problem until we started doing the entire block as
	// one tx.
	//
	// DEPRECATED - block.Stats() should be sufficient to determine the block
	// height, but currently stats are only computed after committing a
	// transaction, therefore cannot be assumed reliable.
	BlockHeight = []byte("BlockHeight")

	// BlockMap is a database bucket containing all of the processed blocks,
	// keyed by their id. This includes blocks that are not currently in the
	// consensus set, and blocks that may not have been fully validated yet.
	BlockMap = []byte("BlockMap")

	// BlockMapV2 is a database bucket containing additional information, including
	// new diff types introduced for SPF-B, for processed blocks, keyed by their id.
	BlockMapV2 = []byte("V2BlockMap")

	// BlockPath is a database bucket containing a mapping from the height of a
	// block to the id of the block at that height. BlockPath only includes
	// blocks in the current path.
	BlockPath = []byte("BlockPath")

	// BucketOak is the database bucket that contains all of the fields related
	// to the oak difficulty adjustment algorithm. The cumulative difficulty and
	// time values are stored for each block id, and then the key "OakInit"
	// contains the value "true" if the oak fields have been properly
	// initialized.
	BucketOak = []byte("Oak")

	// Consistency is a database bucket with a flag indicating whether
	// inconsistencies within the database have been detected.
	Consistency = []byte("Consistency")

	// FileContracts is a database bucket that contains all of the open file
	// contracts.
	FileContracts = []byte("FileContracts")

	// SiacoinOutputs is a database bucket that contains all of the unspent
	// siacoin outputs.
	SiacoinOutputs = []byte("SiacoinOutputs")

	// SiafundOutputs is a database bucket that contains all of the unspent
	// siafund outputs.
	SiafundOutputs = []byte("SiafundOutputs")

	// SiafundPool is a database bucket storing the current value of the
	// siafund pool.
	SiafundPool = []byte("SiafundPool")

	// SiafundHardforkPool is a database bucket storing the value of the
	// siafund pool at the moment of SPF hardfork.
	SiafundHardforkPool = []byte("SiafundHardforkPool")

	// SiafundPoolHistory is a database bucket that stores values of the
	// siafund pool for each block height.
	SiafundPoolHistory = []byte("SiafundPoolHistory")

	// FileContractsOwnership is a database bucket that stores a list of owners
	// for each contract.
	FileContractsOwnership = []byte("FileContractsOwnership")

	// FileContractRanges is a database bucket that stores start and end
	// heights of each contract by its owner.
	FileContractRanges = []byte("FileContractRanges")

	// SiafundBOutputs is a database bucket that stores all existing SPF-B
	// output IDs (only keys are used).
	SiafundBOutputs = []byte("SiafundBOutputs")

	// PoolHistoryHardforkBucket is an utility database bucket that is
	// only used by SPF pool history hardfork code.
	PoolHistoryHardforkBucket = []byte("PoolHistoryHardforkBucket")
)

var (
	// FieldOakInit is a field in BucketOak that gets set to "true" after the
	// oak initialization process has completed.
	FieldOakInit = []byte("OakInit")
)

var (
	// ValueOakInit is the value that the oak init field is set to if the oak
	// difficulty adjustment fields have been correctly initialized.
	ValueOakInit = []byte("true")
)

// createConsensusObjects initializes the consensus portions of the database.
func (cs *ConsensusSet) createConsensusDB(tx *bolt.Tx) error {
	// Enumerate and create the database buckets.
	buckets := [][]byte{
		BlockHeight,
		BlockMap,
		BlockPath,
		Consistency,
		SiacoinOutputs,
		FileContracts,
		SiafundOutputs,
		SiafundPool,
		SiafundHardforkPool,
		SiafundPoolHistory,
		BlockMapV2,
		FileContractsOwnership,
		FileContractRanges,
		SiafundBOutputs,
		PoolHistoryHardforkBucket,
	}
	for _, bucket := range buckets {
		_, err := tx.CreateBucket(bucket)
		if err != nil {
			return err
		}
	}

	// Set the block height to -1, so the genesis block is at height 0.
	blockHeight := tx.Bucket(BlockHeight)
	underflow := types.BlockHeight(0)
	err := blockHeight.Put(BlockHeight, encoding.Marshal(underflow-1))
	if err != nil {
		return err
	}

	// Update the siacoin output diffs map for the genesis block on disk. This
	// needs to happen between the database being opened/initialized and the
	// consensus set hash being calculated
	for _, scod := range cs.blockRoot.SiacoinOutputDiffs {
		commitSiacoinOutputDiff(tx, scod, modules.DiffApply)
	}

	// Set the siafund pool to 0.
	setSiafundPool(tx, types.NewCurrency64(0))

	// Update the siafund output diffs map for the genesis block on disk. This
	// needs to happen between the database being opened/initialized and the
	// consensus set hash being calculated
	for _, sfod := range cs.blockRoot.SiafundOutputDiffs {
		commitSiafundOutputDiff(tx, sfod, modules.DiffApply)
	}

	// Add the miner payout from the genesis block to the delayed siacoin
	// outputs - unspendable, as the unlock hash is blank.
	createDSCOBucket(tx, types.MaturityDelay)
	addDSCO(tx, types.MaturityDelay, cs.blockRoot.Block.MinerPayoutID(0), types.SiacoinOutput{
		Value:      types.CalculateCoinbase(0),
		UnlockHash: types.UnlockHash{},
	})

	// Add the genesis block to the block structures - checksum must be taken
	// after pushing the genesis block into the path.
	pushPath(tx, cs.blockRoot.Block.ID())
	if build.DEBUG {
		cs.blockRoot.ConsensusChecksum = consensusChecksum(tx)
	}
	addBlockMap(tx, &processedBlockV2{processedBlock: cs.blockRoot})
	return nil
}

func claimPerFundInRange(claimStart, startPool, endPool, siafundCount types.Currency, ownersClaimRanges []claimRange) (claim types.SiafundClaim) {
	if claimStart.Cmp(endPool) >= 0 {
		// The claim starts after upper bound, nothing is earned before it.
		return types.SiafundClaim{Total: types.ZeroCurrency, ByOwner: types.ZeroCurrency}
	}
	if claimStart.Cmp(startPool) >= 0 {
		// The claim starts after startPool point, need to truncate.
		startPool = claimStart
	}
	totalEarned := endPool.Sub(startPool)
	ownerTotal := sumClaimRanges(cutClaimRanges(ownersClaimRanges, startPool, endPool))
	claim.Total = totalEarned.Div(siafundCount)
	claim.ByOwner = ownerTotal.Div(siafundCount)
	return claim
}

type hardforkInfo struct {
	pool         types.Currency
	isActivated  bool
	siafundCount types.Currency
}

// ========== 1st hf ======== 2nd hf ======================== fork 2022
// -------------|----------------|--------------------------------|-----------------------------
// 10,000 funds    30,0000 funds          200,000,000 funds           400,000,000 funds
// overall           overall                 overall                      overall
func claimPerFund(startPool, currentPool types.Currency, hardforks []hardforkInfo, ownersClaimRanges []claimRange) types.SiafundClaim {
	totalClaim := types.SiafundClaim{}
	rangeStart := startPool
	count := types.SiafundStates[0].TotalSupply

	for _, hf := range hardforks {
		if !hf.isActivated {
			break
		}
		totalClaim.Add(claimPerFundInRange(startPool, rangeStart, hf.pool, count, ownersClaimRanges))
		rangeStart = hf.pool
		count = hf.siafundCount
	}

	totalClaim.Add(claimPerFundInRange(startPool, rangeStart, currentPool, count, ownersClaimRanges))
	return totalClaim
}

// siafundClaim returns claim by SiafundOutput taking hardforks into account.
func siafundClaim(tx *bolt.Tx, sfoid types.SiafundOutputID, sfo types.SiafundOutput) types.SiafundClaim {
	height := blockHeight(tx)
	hardforks := make([]hardforkInfo, 0, len(types.SiafundStates)-1)
	for _, st := range types.SiafundStates[1:] {
		hf := hardforkInfo{siafundCount: st.TotalSupply}
		if height > st.ActivationHeight {
			hf.isActivated = true
			hf.pool = getSiafundHardforkPool(tx, st.ActivationHeight)
		}
		hardforks = append(hardforks, hf)
	}

	currentPool := getSiafundPool(tx)
	// For SPF-A, we create claim range covering 100% between ClaimStart and now.
	ownersClaimRanges := []claimRange{{start: sfo.ClaimStart, end: currentPool}}
	if isSiafundBOutput(tx, sfoid) {
		// For SPF-B, we replace ownersClaimRanges with actual ranges from consensus.db.
		ownersClaimRanges = claimRanges(tx, currentPool, sfo.UnlockHash)
	}
	claim := claimPerFund(sfo.ClaimStart, currentPool, hardforks, ownersClaimRanges)
	claim.MulCurrency(sfo.Value)
	return claim
}

func heightsWithoutPools(tx *bolt.Tx) (heights []types.BlockHeight) {
	heightsBytes := tx.Bucket(PoolHistoryHardforkBucket).Get(PoolHistoryHardforkBucket)
	err := encoding.Unmarshal(heightsBytes, &heights)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return
}

func storeHeightsWithoutPools(tx *bolt.Tx, heights []types.BlockHeight) {
	b := tx.Bucket(PoolHistoryHardforkBucket)
	err := b.Put(PoolHistoryHardforkBucket, encoding.Marshal(heights))
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// blockHeight returns the height of the blockchain.
func blockHeight(tx *bolt.Tx) types.BlockHeight {
	var height types.BlockHeight
	bh := tx.Bucket(BlockHeight)
	err := encoding.Unmarshal(bh.Get(BlockHeight), &height)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return height
}

// currentBlockID returns the id of the most recent block in the consensus set.
func currentBlockID(tx *bolt.Tx) types.BlockID {
	id, err := getPath(tx, blockHeight(tx))
	if build.DEBUG && err != nil {
		panic(err)
	}
	return id
}

// dbCurrentBlockID is a convenience function allowing currentBlockID to be
// called without a bolt.Tx.
func (cs *ConsensusSet) dbCurrentBlockID() (id types.BlockID) {
	dbErr := cs.db.View(func(tx *bolt.Tx) error {
		id = currentBlockID(tx)
		return nil
	})
	if dbErr != nil {
		panic(dbErr)
	}
	return id
}

// currentProcessedBlock returns the most recent block in the consensus set.
func currentProcessedBlock(tx *bolt.Tx) *processedBlockV2 {
	pb, err := getBlockMap(tx, currentBlockID(tx))
	if build.DEBUG && err != nil {
		panic(err)
	}
	return pb
}

// getBlockMap returns a processed block with the input id.
func getBlockMap(tx *bolt.Tx, id types.BlockID) (*processedBlockV2, error) {
	return getBlockMapWithTx(boltTxWrapper{tx}, id, nil)
}

func unmarshal(marshaler marshaler, data []byte, obj interface{}) error {
	if marshaler != nil {
		return marshaler.Unmarshal(data, obj)
	}
	return encoding.Unmarshal(data, obj)
}

func getBlockMapWithTx(tx dbTx, id types.BlockID, marshaler marshaler) (*processedBlockV2, error) {
	// Look up the encoded block.
	pbBytes := tx.Bucket(BlockMap).Get(id[:])
	if pbBytes == nil {
		return nil, errNilItem
	}

	// Decode the block - should never fail.
	var pb processedBlock
	err := unmarshal(marshaler, pbBytes, &pb)
	if build.DEBUG && err != nil {
		return nil, err
	}

	// Check if there is V2 part of processedBlock.
	pbBytesV2 := tx.Bucket(BlockMapV2).Get(id[:])
	if pbBytesV2 == nil {
		return &processedBlockV2{processedBlock: pb}, nil
	}

	var pbd processedBlockV2Diffs
	err = unmarshal(marshaler, pbBytesV2, &pbd)
	if build.DEBUG && err != nil {
		return nil, err
	}

	return &processedBlockV2{processedBlock: pb, processedBlockV2Diffs: pbd}, nil
}

// addBlockMap adds a processed block to the block map.
func addBlockMap(tx *bolt.Tx, pb *processedBlockV2) {
	id := pb.Block.ID()
	err := tx.Bucket(BlockMap).Put(id[:], encoding.Marshal(pb.processedBlock))
	if build.DEBUG && err != nil {
		panic(err)
	}
	err = tx.Bucket(BlockMapV2).Put(id[:], encoding.Marshal(pb.processedBlockV2Diffs))
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// getPath returns the block id at 'height' in the block path.
func getPath(tx *bolt.Tx, height types.BlockHeight) (id types.BlockID, err error) {
	idBytes := tx.Bucket(BlockPath).Get(encoding.Marshal(height))
	if idBytes == nil {
		return types.BlockID{}, errNilItem
	}

	err = encoding.Unmarshal(idBytes, &id)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return id, nil
}

// pushPath adds a block to the BlockPath at current height + 1.
func pushPath(tx *bolt.Tx, bid types.BlockID) {
	// Fetch and update the block height.
	bh := tx.Bucket(BlockHeight)
	heightBytes := bh.Get(BlockHeight)
	var oldHeight types.BlockHeight
	err := encoding.Unmarshal(heightBytes, &oldHeight)
	if build.DEBUG && err != nil {
		panic(err)
	}
	newHeightBytes := encoding.Marshal(oldHeight + 1)
	err = bh.Put(BlockHeight, newHeightBytes)
	if build.DEBUG && err != nil {
		panic(err)
	}

	// Add the block to the block path.
	bp := tx.Bucket(BlockPath)
	err = bp.Put(newHeightBytes, bid[:])
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// popPath removes a block from the "end" of the chain, i.e. the block
// with the largest height.
func popPath(tx *bolt.Tx) {
	// Fetch and update the block height.
	bh := tx.Bucket(BlockHeight)
	oldHeightBytes := bh.Get(BlockHeight)
	var oldHeight types.BlockHeight
	err := encoding.Unmarshal(oldHeightBytes, &oldHeight)
	if build.DEBUG && err != nil {
		panic(err)
	}
	newHeightBytes := encoding.Marshal(oldHeight - 1)
	err = bh.Put(BlockHeight, newHeightBytes)
	if build.DEBUG && err != nil {
		panic(err)
	}

	// Remove the block from the path - make sure to remove the block at
	// oldHeight.
	bp := tx.Bucket(BlockPath)
	err = bp.Delete(oldHeightBytes)
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// isSiacoinOutput returns true if there is a siacoin output of that id in the
// database.
func isSiacoinOutput(tx *bolt.Tx, id types.SiacoinOutputID) bool {
	bucket := tx.Bucket(SiacoinOutputs)
	sco := bucket.Get(id[:])
	return sco != nil
}

// getSiacoinOutput fetches a siacoin output from the database. An error is
// returned if the siacoin output does not exist.
func getSiacoinOutput(tx *bolt.Tx, id types.SiacoinOutputID) (types.SiacoinOutput, error) {
	scoBytes := tx.Bucket(SiacoinOutputs).Get(id[:])
	if scoBytes == nil {
		return types.SiacoinOutput{}, errNilItem
	}
	var sco types.SiacoinOutput
	err := encoding.Unmarshal(scoBytes, &sco)
	if err != nil {
		return types.SiacoinOutput{}, err
	}
	return sco, nil
}

// addSiacoinOutput adds a siacoin output to the database. An error is returned
// if the siacoin output is already in the database.
func addSiacoinOutput(tx *bolt.Tx, id types.SiacoinOutputID, sco types.SiacoinOutput) {
	// While this is not supposed to be allowed, there's a bug in the consensus
	// code which means that earlier versions have accetped 0-value outputs
	// onto the blockchain. A hardfork to remove 0-value outputs will fix this,
	// and that hardfork is planned, but not yet.
	/*
		if build.DEBUG && sco.Value.IsZero() {
			panic("discovered a zero value scprimecoin output")
		}
	*/
	siacoinOutputs := tx.Bucket(SiacoinOutputs)
	// Sanity check - should not be adding an item that exists.
	if build.DEBUG && siacoinOutputs.Get(id[:]) != nil {
		panic("repeat scprimecoin output")
	}
	err := siacoinOutputs.Put(id[:], encoding.Marshal(sco))
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// removeSiacoinOutput removes a siacoin output from the database. An error is
// returned if the siacoin output is not in the database prior to removal.
func removeSiacoinOutput(tx *bolt.Tx, id types.SiacoinOutputID) {
	scoBucket := tx.Bucket(SiacoinOutputs)
	// Sanity check - should not be removing an item that is not in the db.
	if build.DEBUG && scoBucket.Get(id[:]) == nil {
		panic("nil scprimecoin output")
	}
	err := scoBucket.Delete(id[:])
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// getFileContract fetches a file contract from the database, returning an
// error if it is not there.
func getFileContract(tx *bolt.Tx, id types.FileContractID) (fc types.FileContract, err error) {
	fcBytes := tx.Bucket(FileContracts).Get(id[:])
	if fcBytes == nil {
		return types.FileContract{}, errNilItem
	}
	err = encoding.Unmarshal(fcBytes, &fc)
	if err != nil {
		return types.FileContract{}, err
	}
	return fc, nil
}

// addFileContract adds a file contract to the database. An error is returned
// if the file contract is already in the database.
func addFileContract(tx *bolt.Tx, id types.FileContractID, fc types.FileContract) {
	// Add the file contract to the database.
	fcBucket := tx.Bucket(FileContracts)
	// Sanity check - should not be adding a zero-payout file contract.
	if build.DEBUG && fc.Payout.IsZero() {
		panic("adding zero-payout file contract")
	}
	// Sanity check - should not be adding a file contract already in the db.
	if build.DEBUG && fcBucket.Get(id[:]) != nil {
		panic("repeat file contract")
	}
	err := fcBucket.Put(id[:], encoding.Marshal(fc))
	if build.DEBUG && err != nil {
		panic(err)
	}

	// Add an entry for when the file contract expires.
	expirationBucketID := append(prefixFCEX, encoding.Marshal(fc.WindowEnd)...)
	expirationBucket, err := tx.CreateBucketIfNotExists(expirationBucketID)
	if build.DEBUG && err != nil {
		panic(err)
	}
	err = expirationBucket.Put(id[:], []byte{})
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// removeFileContract removes a file contract from the database.
func removeFileContract(tx *bolt.Tx, id types.FileContractID) {
	// Delete the file contract entry.
	fcBucket := tx.Bucket(FileContracts)
	fcBytes := fcBucket.Get(id[:])
	// Sanity check - should not be removing a file contract not in the db.
	if build.DEBUG && fcBytes == nil {
		panic("nil file contract")
	}
	fcBytesCopy := make([]byte, len(fcBytes))
	copy(fcBytesCopy, fcBytes)
	err := fcBucket.Delete(id[:])
	if build.DEBUG && err != nil {
		panic(err)
	}

	// Delete the entry for the file contract's expiration. The portion of
	// 'fcBytes' used to determine the expiration bucket id is the
	// byte-representation of the file contract window end, which always
	// appears at bytes 48-56.
	expirationBucketID := append(prefixFCEX, fcBytesCopy[48:56]...)
	expirationBucket := tx.Bucket(expirationBucketID)
	expirationBytes := expirationBucket.Get(id[:])
	if expirationBytes == nil {
		panic(errNilItem)
	}
	err = expirationBucket.Delete(id[:])
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// The address of the devs.
var devAddr = types.UnlockHash{243, 113, 199, 11, 206, 158, 184,
	151, 156, 213, 9, 159, 89, 158, 196, 228, 252, 177, 78, 10,
	252, 243, 31, 151, 145, 224, 62, 100, 150, 164, 192, 179}

// getSiafundOutput fetches a siafund output from the database. An error is
// returned if the siafund output does not exist.
func getSiafundOutput(tx *bolt.Tx, id types.SiafundOutputID) (types.SiafundOutput, error) {
	sfoBytes := tx.Bucket(SiafundOutputs).Get(id[:])
	if sfoBytes == nil {
		return types.SiafundOutput{}, errNilItem
	}
	var sfo types.SiafundOutput
	err := encoding.Unmarshal(sfoBytes, &sfo)
	if err != nil {
		return types.SiafundOutput{}, err
	}
	gsa := types.GenesisSiafundAllocation
	if sfo.UnlockHash == gsa[len(gsa)-1].UnlockHash && blockHeight(tx) > 10e3 {
		sfo.UnlockHash = devAddr
	}
	return sfo, nil
}

// addSiafundBOutput marks `id` as SPF-B output.
func addSiafundBOutput(tx *bolt.Tx, id types.SiafundOutputID) {
	err := tx.Bucket(SiafundBOutputs).Put(id[:], []byte{})
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// removeSiafundBOutput unmarks `id` as SPF-B output.
func removeSiafundBOutput(tx *bolt.Tx, id types.SiafundOutputID) {
	err := tx.Bucket(SiafundBOutputs).Delete(id[:])
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// isSiafundBOutput checks if SPF output specified by `id` is SPF-B.
func isSiafundBOutput(tx *bolt.Tx, id types.SiafundOutputID) bool {
	val := tx.Bucket(SiafundBOutputs).Get(id[:])
	if val == nil {
		return false
	}
	return true
}

// addSiafundOutput adds a siafund output to the database. An error is returned
// if the siafund output is already in the database.
func addSiafundOutput(tx *bolt.Tx, id types.SiafundOutputID, sfo types.SiafundOutput) {
	siafundOutputs := tx.Bucket(SiafundOutputs)
	// Sanity check - should not be adding a siafund output with a value of
	// zero.
	if build.DEBUG && sfo.Value.IsZero() {
		panic("zero value scprimefund being added")
	}
	// Sanity check - should not be adding an item already in the db.
	if build.DEBUG && siafundOutputs.Get(id[:]) != nil {
		panic("repeat scprimefund output")
	}
	err := siafundOutputs.Put(id[:], encoding.Marshal(sfo))
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// removeSiafundOutput removes a siafund output from the database. An error is
// returned if the siafund output is not in the database prior to removal.
func removeSiafundOutput(tx *bolt.Tx, id types.SiafundOutputID) {
	sfoBucket := tx.Bucket(SiafundOutputs)
	if build.DEBUG && sfoBucket.Get(id[:]) == nil {
		panic("nil scprimefund output")
	}
	err := sfoBucket.Delete(id[:])
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// getSiafundHardforkPool returns the value of the siafund pool at the
// moment of SPF hardfork.
func getSiafundHardforkPool(tx *bolt.Tx, height types.BlockHeight) (pool types.Currency) {
	bucket := tx.Bucket(SiafundHardforkPool)
	if bucket == nil {
		// Should never happen.
		_, _ = tx.CreateBucket(SiafundHardforkPool)
		return types.ZeroCurrency
	}
	heightBytes := encoding.EncUint64(uint64(height))
	poolBytes := bucket.Get(heightBytes)
	if poolBytes == nil && height == types.SpfHardforkHeight {
		// Different key was used for the first hardfork before v1.5.0.1
		poolBytes = bucket.Get(SiafundHardforkPool)
	}
	// An error should only be returned if the object stored in the siafund
	// pool bucket is either unavailable or otherwise malformed. As this is a
	// developer error, a panic is appropriate.
	err := encoding.Unmarshal(poolBytes, &pool)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return pool
}

// setSiafundHardforkPool sets the siafund hardfork pool value at block height.
func setSiafundHardforkPool(tx *bolt.Tx, c types.Currency, height types.BlockHeight) {
	bucket := tx.Bucket(SiafundHardforkPool)
	if bucket == nil {
		bucket, _ = tx.CreateBucket(SiafundHardforkPool)
	}
	heightBytes := encoding.EncUint64(uint64(height))
	err := bucket.Put(heightBytes, encoding.Marshal(c))
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// getSiafundPool returns the current value of the siafund pool. No error is
// returned as the siafund pool should always be available.
func getSiafundPool(tx *bolt.Tx) (pool types.Currency) {
	bucket := tx.Bucket(SiafundPool)
	poolBytes := bucket.Get(SiafundPool)
	// An error should only be returned if the object stored in the siafund
	// pool bucket is either unavailable or otherwise malformed. As this is a
	// developer error, a panic is appropriate.
	err := encoding.Unmarshal(poolBytes, &pool)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return pool
}

// setSiafundPool updates the saved siafund pool on disk
func setSiafundPool(tx *bolt.Tx, c types.Currency) {
	err := tx.Bucket(SiafundPool).Put(SiafundPool, encoding.Marshal(c))
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// getSiafundPoolAtHeight returns siafund pool value at specified height.
func getSiafundPoolAtHeight(tx *bolt.Tx, height types.BlockHeight) (pool types.Currency, err error) {
	poolBytes := tx.Bucket(SiafundPoolHistory).Get(encoding.Marshal(height))
	if poolBytes == nil {
		return types.ZeroCurrency, errNilItem
	}
	err = encoding.Unmarshal(poolBytes, &pool)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return pool, nil
}

// setSiafundHistoricalPool sets historical siafund pool value.
func setSiafundHistoricalPool(tx *bolt.Tx, c types.Currency, height types.BlockHeight) {
	err := tx.Bucket(SiafundPoolHistory).Put(encoding.Marshal(height), encoding.Marshal(c))
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// removeSiafundHistoricalPool removes historical siafund pool value.
func removeSiafundHistoricalPool(tx *bolt.Tx, height types.BlockHeight) {
	err := tx.Bucket(SiafundPoolHistory).Delete(encoding.Marshal(height))
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// addDSCO adds a delayed siacoin output to the consnesus set.
func addDSCO(tx *bolt.Tx, bh types.BlockHeight, id types.SiacoinOutputID, sco types.SiacoinOutput) {
	// Sanity check - dsco should never have a value of zero.
	// An error in the consensus code means sometimes there are 0-value dscos
	// in the blockchain. A hardfork will fix this.
	/*
		if build.DEBUG && sco.Value.IsZero() {
			panic("zero-value dsco being added")
		}
	*/
	// Sanity check - output should not already be in the full set of outputs.
	if build.DEBUG && tx.Bucket(SiacoinOutputs).Get(id[:]) != nil {
		panic("dsco already in output set")
	}
	dscoBucketID := append(prefixDSCO, encoding.EncUint64(uint64(bh))...)
	dscoBucket := tx.Bucket(dscoBucketID)
	// Sanity check - should not be adding an item already in the db.
	if build.DEBUG && dscoBucket.Get(id[:]) != nil {
		panic(errRepeatInsert)
	}
	err := dscoBucket.Put(id[:], encoding.Marshal(sco))
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// removeDSCO removes a delayed siacoin output from the consensus set.
func removeDSCO(tx *bolt.Tx, bh types.BlockHeight, id types.SiacoinOutputID) {
	bucketID := append(prefixDSCO, encoding.Marshal(bh)...)
	// Sanity check - should not remove an item not in the db.
	dscoBucket := tx.Bucket(bucketID)
	if build.DEBUG && dscoBucket.Get(id[:]) == nil {
		panic("nil dsco")
	}
	err := dscoBucket.Delete(id[:])
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// createDSCOBucket creates a bucket for the delayed siacoin outputs at the
// input height.
func createDSCOBucket(tx *bolt.Tx, bh types.BlockHeight) {
	bucketID := append(prefixDSCO, encoding.Marshal(bh)...)
	_, err := tx.CreateBucket(bucketID)
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// deleteDSCOBucket deletes the bucket that held a set of delayed siacoin
// outputs.
func deleteDSCOBucket(tx *bolt.Tx, bh types.BlockHeight) {
	// Delete the bucket.
	bucketID := append(prefixDSCO, encoding.Marshal(bh)...)
	bucket := tx.Bucket(bucketID)
	if build.DEBUG && bucket == nil {
		panic(errNilBucket)
	}

	// TODO: Check that the bucket is empty. Using Stats() does not work at the
	// moment, as there is an error in the boltdb code.

	err := tx.DeleteBucket(bucketID)
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// FileContractOwnership contains a list of contract's owners and its start height.
type FileContractOwnership struct {
	Start  types.BlockHeight
	Owners []types.UnlockHash
}

// addFileContractOwnership adds ownership info by contractID.
func addFileContractOwnership(tx *bolt.Tx, contractID types.FileContractID, ownership FileContractOwnership) {
	err := tx.Bucket(FileContractsOwnership).Put(contractID[:], encoding.Marshal(ownership))
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// removeFileContractOwnership removes ownership info by contractID.
func removeFileContractOwnership(tx *bolt.Tx, contractID types.FileContractID) {
	err := tx.Bucket(FileContractsOwnership).Delete(contractID[:])
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// getFileContractOwnership returns ownership info by contractID.
func getFileContractOwnership(tx *bolt.Tx, contractID types.FileContractID) (FileContractOwnership, error) {
	var ownership FileContractOwnership
	ownershipBytes := tx.Bucket(FileContractsOwnership).Get(contractID[:])
	if ownershipBytes == nil {
		return FileContractOwnership{}, errNilItem
	}
	err := encoding.Unmarshal(ownershipBytes, &ownership)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return ownership, nil
}

// FileContractRange is a block height range specifying contract lifetime.
type FileContractRange struct {
	Start types.BlockHeight
	End   types.BlockHeight
}

// Marshal implements serialisation for FileContractRange structure.
func (fcr FileContractRange) Marshal() []byte {
	fcrBytes := make([]byte, binary.MaxVarintLen64*2)
	startLen := binary.PutUvarint(fcrBytes, uint64(fcr.Start))
	endLen := binary.PutUvarint(fcrBytes[startLen:], uint64(fcr.End))
	return fcrBytes[:startLen+endLen]
}

// Unmarshal implements parsing for FileContractRange structure.
func (fcr *FileContractRange) Unmarshal(data []byte) int {
	start, startLen := binary.Uvarint(data)
	end, endLen := binary.Uvarint(data[startLen:])
	fcr.Start = types.BlockHeight(start)
	fcr.End = types.BlockHeight(end)
	return startLen + endLen
}

// Equals checks if two file contract ranges are equal.
func (fcr FileContractRange) Equals(fcr2 FileContractRange) bool {
	return fcr.Start == fcr2.Start && fcr.End == fcr2.End
}

// addFileContractRange adds file contract range to each of `owners`.
func addFileContractRange(tx *bolt.Tx, owners []types.UnlockHash, r FileContractRange) {
	newRangeBytes := r.Marshal()
	for _, owner := range owners {
		rangesBytes := tx.Bucket(FileContractRanges).Get(owner[:])
		rangesBytesCopy := make([]byte, len(rangesBytes))
		copy(rangesBytesCopy, rangesBytes)
		err := tx.Bucket(FileContractRanges).Put(owner[:], append(rangesBytesCopy, newRangeBytes...))
		if build.DEBUG && err != nil {
			panic(err)
		}
	}
}

func unmarshalRanges(data []byte) (res []FileContractRange, offsets []int, err error) {
	bytesRead := 0
	for bytesRead != len(data) {
		offsets = append(offsets, bytesRead)
		fcr := FileContractRange{}
		bytesRead += fcr.Unmarshal(data[bytesRead:])
		res = append(res, fcr)
	}
	return res, offsets, nil
}

// nonOverlappingRanges takes a list of ranges and combines them returning
// non-overlapping ranges covering all the initial ranges.
func nonOverlappingRanges(ranges []FileContractRange) []FileContractRange {
	// Put all bound points from ranges into the map to merge duplicates.
	type pointInfo struct {
		closing int
		opening int
	}
	boundsMap := make(map[types.BlockHeight]pointInfo)
	for _, r := range ranges {
		start := boundsMap[r.Start]
		start.opening++
		boundsMap[r.Start] = start
		end := boundsMap[r.End]
		end.closing++
		boundsMap[r.End] = end
	}

	// Convert map into sorted list of points.
	type rangeBound struct {
		point types.BlockHeight
		pointInfo
	}
	bounds := make([]rangeBound, 0, len(boundsMap))
	for height, pi := range boundsMap {
		bounds = append(bounds, rangeBound{point: height, pointInfo: pi})
	}
	sort.Slice(bounds, func(i, j int) bool {
		return bounds[i].point < bounds[j].point
	})

	// Count open ranges and build final ranges.
	openRanges := 0
	finalRanges := make([]FileContractRange, 0, len(bounds))
	var currentRange *FileContractRange
	for _, p := range bounds {
		if p.opening > 0 {
			openRanges += p.opening
		}
		if p.closing > 0 {
			openRanges -= p.closing
		}
		if openRanges > 0 && currentRange == nil {
			currentRange = &FileContractRange{Start: p.point}
		}
		if openRanges == 0 && currentRange != nil {
			currentRange.End = p.point
			finalRanges = append(finalRanges, *currentRange)
			currentRange = nil
		}
	}
	return finalRanges
}

type claimRange struct {
	start types.Currency
	end   types.Currency
}

func (cr claimRange) diff() types.Currency {
	return cr.end.Sub(cr.start)
}

func claimRanges(tx *bolt.Tx, currentPool types.Currency, owner types.UnlockHash) []claimRange {
	contractRanges := nonOverlappingRanges(getFileContractRanges(tx, owner))
	claims := make([]claimRange, 0, len(contractRanges))
	for _, cr := range contractRanges {
		startPool, err := getSiafundPoolAtHeight(tx, cr.Start)
		if err == errNilItem {
			startPool = currentPool
		} else if build.DEBUG && err != nil {
			panic(err)
		}

		endPool, err := getSiafundPoolAtHeight(tx, cr.End)
		if err == errNilItem {
			endPool = currentPool
		} else if build.DEBUG && err != nil {
			panic(err)
		}

		claims = append(claims, claimRange{start: startPool, end: endPool})
	}
	return claims
}

func cutClaimRanges(sortedRanges []claimRange, start, end types.Currency) []claimRange {
	res := make([]claimRange, 0, len(sortedRanges))
	for _, r := range sortedRanges {
		curRange := claimRange{start: r.start, end: r.end}
		if curRange.end.Cmp(start) < 0 {
			continue
		}
		if curRange.start.Cmp(start) < 0 {
			curRange.start = start
		}
		if curRange.start.Cmp(end) > 0 {
			break
		}
		if curRange.end.Cmp(end) > 0 {
			curRange.end = end
		}
		res = append(res, curRange)
	}
	return res
}

func sumClaimRanges(ranges []claimRange) types.Currency {
	sum := types.ZeroCurrency
	for _, r := range ranges {
		sum = sum.Add(r.diff())
	}
	return sum
}

// getFileContractRanges retrieves all historical file contract ranges by owner.
func getFileContractRanges(tx *bolt.Tx, owner types.UnlockHash) []FileContractRange {
	rangesBytes := tx.Bucket(FileContractRanges).Get(owner[:])
	if rangesBytes == nil {
		return []FileContractRange{}
	}
	ranges, _, err := unmarshalRanges(rangesBytes)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return ranges
}

// cutRangeInplace removes range `r` from rangesBytes and returns updated rangesBytes.
// If r is not in rangesBytes, slice is returned unchanged.
func cutRangeInplace(rangesBytes []byte, r FileContractRange) (bool, []byte) {
	ranges, layouts, err := unmarshalRanges(rangesBytes)
	if build.DEBUG && err != nil {
		panic(err)
	}
	// Find and remove range `r` from ranges.
	foundAt := -1
	for i, dbRange := range ranges {
		if r.Equals(dbRange) {
			foundAt = i
			break
		}
	}
	if foundAt == -1 {
		// Not found.
		return false, rangesBytes
	}
	removedStart := layouts[foundAt]
	var removedEnd int
	if foundAt == len(layouts)-1 {
		removedEnd = len(rangesBytes)
	} else {
		removedEnd = layouts[foundAt+1]
	}
	rangesBytes = append(rangesBytes[:removedStart], rangesBytes[removedEnd:]...)
	return true, rangesBytes
}

// removeFileContractRange removes file contract range `r` from each owner.
func removeFileContractRange(tx *bolt.Tx, owners []types.UnlockHash, r FileContractRange) {
	for _, owner := range owners {
		rangesBytes := tx.Bucket(FileContractRanges).Get(owner[:])
		rangesBytesCopy := make([]byte, len(rangesBytes))
		copy(rangesBytesCopy, rangesBytes)
		// Optimisation: do not encode ranges back, remove chunk directly from `rangesBytes`.
		found, rangesBytesCopy := cutRangeInplace(rangesBytesCopy, r)
		if !found {
			continue
		}
		err := tx.Bucket(FileContractRanges).Put(owner[:], rangesBytesCopy)
		if build.DEBUG && err != nil {
			panic(err)
		}
	}
}
