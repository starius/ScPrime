package consensus

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/persist"

	bolt "go.etcd.io/bbolt"
)

var (
	// DatabaseBucketsMayNotExist contains buckets which did not exist
	// in previous versions.
	DatabaseBucketsMayNotExist = [][]byte{
		SiafundHardforkPool,
		SiafundPoolHistory,
		FileContractsOwnership,
		FileContractRanges,
		SiafundBOutputs,
		BlockMapV2,
		PoolHistoryHardforkBucket,
	}
)

const (
	// DatabaseFilename contains the filename of the database that will be used
	// when managing consensus.
	DatabaseFilename = modules.ConsensusDir + ".db"
	logFile          = modules.ConsensusDir + ".log"
)

// loadDB pulls all the blocks that have been saved to disk into memory, using
// them to fill out the ConsensusSet.
func (cs *ConsensusSet) loadDB() error {
	// Open the database - a new bolt database will be created if none exists.
	err := cs.openDB(filepath.Join(cs.persistDir, DatabaseFilename))
	if err != nil {
		return err
	}

	// Walk through initialization for ScPrime.
	return cs.db.Update(func(tx *bolt.Tx) error {
		// Check if the database has been initialized.
		err = cs.initDB(tx)
		if err != nil {
			return err
		}

		// For backward storage compatibility, we create some buckets here, if they do not yet exist.
		for _, bucket := range DatabaseBucketsMayNotExist {
			_, err := tx.CreateBucketIfNotExists(bucket)
			if err != nil {
				return err
			}
		}

		// Check the initialization of the oak difficulty adjustment fields, and
		// create them if they do not exist. This is separate from 'initDB'
		// because older consensus databases will have completed the 'initDB'
		// process but will not have the oak difficulty adjustment fields, so a
		// scan will be needed to add and update them.
		err = cs.initOak(tx)
		if err != nil {
			return err
		}

		// Check that the genesis block is correct - typically only incorrect
		// in the event of developer binaries vs. release binaires.
		genesisID, err := getPath(tx, 0)
		if build.DEBUG && err != nil {
			panic(err)
		}
		if genesisID != cs.blockRoot.Block.ID() {
			return errors.New("blockchain has wrong genesis block")
		}
		return nil
	})
}

// initPersist initializes the persistence structures of the consensus set, in
// particular loading the database and preparing to manage subscribers.
func (cs *ConsensusSet) initPersist() error {
	// Create the consensus directory.
	err := os.MkdirAll(cs.persistDir, 0700)
	if err != nil {
		return err
	}

	// Initialize the logger.
	cs.log, err = persist.NewFileLogger(filepath.Join(cs.persistDir, logFile))
	if err != nil {
		return err
	}
	// Set up closing the logger.
	err = cs.tg.AfterStop(func() error {
		err := cs.log.Close()
		if err != nil {
			// State of the logger is unknown, a println will suffice.
			fmt.Println("Error shutting down consensus set logger:", err)
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Try to load an existing database from disk - a new one will be created
	// if one does not exist.
	err = cs.loadDB()
	if err != nil {
		return err
	}

	// Set up the closing of the database.
	err = cs.tg.AfterStop(func() error {
		err := cs.db.Close()
		if err != nil {
			cs.log.Println("ERROR: Unable to close consensus set database at shutdown:", err)
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
