package tokenstate

import (
	"os"
	"path"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type sectorsDBer interface {
	Put(key, value []byte) error
	Delete(key []byte) error
	BatchDelete(bytesPrefix []byte) error
}

type sectorsDB struct {
	db *leveldb.DB
}

func newSectorsDB(dir string) *sectorsDB {
	dbDir := path.Join(dir, "levelDB")
	// remove all files from level DB dir and create new folder
	// all sectors will be upload from events
	_ = os.RemoveAll(dbDir)
	_ = os.Mkdir(dbDir, 0700)
	db, _ := leveldb.OpenFile(dbDir, nil)
	return &sectorsDB{db: db}
}

func (s *sectorsDB) Put(key, value []byte) error {
	return s.db.Put(key, value, nil)
}

func (s *sectorsDB) Delete(key []byte) error {
	return s.db.Delete(key, nil)
}

func (s *sectorsDB) BatchDelete(bytesPrefix []byte) error {
	iter := s.db.NewIterator(util.BytesPrefix(bytesPrefix), nil)

	for iter.Next() {
		_ = s.db.Delete(iter.Key(), nil)
	}
	iter.Release()
	return iter.Error()
}
