package tokenstate

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/types"
)

type sectorsDB struct {
	db *leveldb.DB
}

func newSectorsDB(dir string) (*sectorsDB, error) {
	dbDir := filepath.Join(dir, "level_db")
	// remove all files from level DB dir and create new folder
	// all sectors will be upload from events.
	err := os.RemoveAll(dbDir)
	if err != nil {
		return nil, fmt.Errorf("failed to remove old level DB directory: %w", err)
	}
	err = os.Mkdir(dbDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create new level DB directory: %w", err)
	}
	db, err := leveldb.OpenFile(dbDir, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open level DB directory: %w", err)
	}
	return &sectorsDB{db: db}, nil
}

func (s *sectorsDB) Get(tokenID types.TokenID) ([]crypto.Hash, error) {
	var sectors []crypto.Hash
	iter := s.db.NewIterator(util.BytesPrefix(tokenID.Bytes()), nil)

	for iter.Next() {
		sectors = append(sectors, crypto.ConvertBytesToHash(bytes.TrimPrefix(iter.Key(), tokenID.Bytes())))
	}
	iter.Release()
	return sectors, iter.Error()
}

func (s *sectorsDB) GetLimited(tokenID types.TokenID, pageID string, limit int) (sectorIDs []crypto.Hash, nextPageID string, err error) {
	if limit <= 0 {
		panic(fmt.Errorf("invalid request. limit want: >= 0, have: %d", limit))
	}

	rangeOpts, err := s.createRangeOptsPrefixStart(tokenID, pageID)
	if err != nil {
		return nil, "", fmt.Errorf("create range opts: %w", err)
	}

	var sectors []crypto.Hash
	iter := s.db.NewIterator(rangeOpts, nil)
	for iter.Next() {
		// Start from pageID.
		sectorID := crypto.ConvertBytesToHash(bytes.TrimPrefix(iter.Key(), tokenID.Bytes()))

		// Limit exceeded, set nextPageID and break.
		if limit == 0 {
			nextPageID = sectorID.String()
			break
		}

		// Add sectors to response.
		sectors = append(sectors, sectorID)
		limit--
	}

	iter.Release()
	return sectors, nextPageID, iter.Error()
}

func (s *sectorsDB) Put(tokenID types.TokenID, sectorID crypto.Hash) error {
	return s.db.Put(createDBKey(tokenID, sectorID), nil, nil)
}

func (s *sectorsDB) HasSectors(tokenID types.TokenID, sectorIDs []crypto.Hash) (bool, error) {
	for _, sectorID := range sectorIDs {
		ok, err := s.db.Has(createDBKey(tokenID, sectorID), nil)
		if err != nil {
			return false, fmt.Errorf("check has sector: %w", err)
		}
		if !ok {
			return false, nil
		}
	}

	return true, nil
}

func (s *sectorsDB) BatchDeleteSpecific(tokenID types.TokenID, sectorIDs []crypto.Hash) error {
	batch := new(leveldb.Batch)
	for _, sectorID := range sectorIDs {
		batch.Delete(createDBKey(tokenID, sectorID))
	}

	return s.db.Write(batch, nil)
}

func (s *sectorsDB) BatchDeleteAll(tokenID types.TokenID) error {
	iter := s.db.NewIterator(util.BytesPrefix(tokenID.Bytes()), nil)
	batch := new(leveldb.Batch)

	for iter.Next() {
		batch.Delete(iter.Key())
	}
	err := s.db.Write(batch, nil)
	if err != nil {
		return err
	}
	iter.Release()
	return iter.Error()
}

func (s *sectorsDB) Close() error {
	return s.db.Close()
}

// createRangeOptsPrefixStart creates range opts for db iterator. TokenID used as prefix within which range happens,
// use pageID as start point.
func (s *sectorsDB) createRangeOptsPrefixStart(tokenID types.TokenID, pageID string) (*util.Range, error) {
	// Create start point page id.
	rangeOpts := util.BytesPrefix(tokenID.Bytes())

	// If start from middle.
	if pageID != "" {
		pageIDHash := &crypto.Hash{}
		if err := pageIDHash.LoadString(pageID); err != nil {
			return nil, fmt.Errorf("load string: %w", err)
		}

		// Check if such key exists in database.
		ok, err := s.db.Has(createDBKey(tokenID, *pageIDHash), nil)
		if err != nil {
			return nil, fmt.Errorf("check has sector: %w", err)
		}
		if !ok {
			return nil, fmt.Errorf("page id does not exists")
		}

		// Set range start.
		rangeOpts.Start = createDBKey(tokenID, *pageIDHash)
	}

	return rangeOpts, nil
}

func createDBKey(tokenID types.TokenID, sectorID crypto.Hash) []byte {
	buf := bytes.Buffer{}
	buf.Write(tokenID.Bytes())
	buf.Write(sectorID.Bytes())
	return buf.Bytes()
}
