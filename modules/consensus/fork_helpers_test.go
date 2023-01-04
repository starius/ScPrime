package consensus

import bolt "go.etcd.io/bbolt"

// dbBacktrackToCurrentPath is a convenience function to call
// backtrackToCurrentPath without a bolt.Tx.
func (cs *ConsensusSet) dbBacktrackToCurrentPath(pb *processedBlockV2) (pbs []*processedBlockV2) {
	_ = cs.db.Update(func(tx *bolt.Tx) error {
		pbs = backtrackToCurrentPath(tx, pb)
		return nil
	})
	return pbs
}

// dbRevertToNode is a convenience function to call revertToBlock without a
// bolt.Tx.
func (cs *ConsensusSet) dbRevertToNode(pb *processedBlockV2) (pbs []*processedBlockV2) {
	_ = cs.db.Update(func(tx *bolt.Tx) error {
		pbs = cs.revertToBlock(tx, pb)
		return nil
	})
	return pbs
}

// dbForkBlockchain is a convenience function to call forkBlockchain without a
// bolt.Tx.
func (cs *ConsensusSet) dbForkBlockchain(pb *processedBlockV2) (revertedBlocks, appliedBlocks []*processedBlockV2, err error) {
	updateErr := cs.db.Update(func(tx *bolt.Tx) error {
		revertedBlocks, appliedBlocks, err = cs.forkBlockchain(tx, pb)
		return nil
	})
	if updateErr != nil {
		panic(updateErr)
	}
	return revertedBlocks, appliedBlocks, err
}
