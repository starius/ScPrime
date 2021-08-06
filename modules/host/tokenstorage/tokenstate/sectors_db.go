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
		sectors = append(sectors, crypto.HashBytes(bytes.TrimPrefix(iter.Key(), tokenID.Bytes())))
	}
	iter.Release()
	return sectors, iter.Error()
}

func (s *sectorsDB) Put(tokenID types.TokenID, sectorID crypto.Hash) error {
	buf := bytes.Buffer{}
	buf.Write(tokenID.Bytes())
	buf.Write(sectorID.Bytes())
	return s.db.Put(buf.Bytes(), nil, nil)
}

func (s *sectorsDB) Delete(tokenID types.TokenID, sectorID crypto.Hash) error {
	buf := bytes.Buffer{}
	buf.Write(tokenID.Bytes())
	buf.Write(sectorID.Bytes())
	return s.db.Delete(buf.Bytes(), nil)
}

func (s *sectorsDB) BatchDelete(tokenID types.TokenID) error {
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
