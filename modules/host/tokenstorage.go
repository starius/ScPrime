package host

import (
	"encoding/binary"

	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"

	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"
)

// TODO: the current version of tokens storage does not support reverting blocks.
// If contracts related to `TopUpToken` RPC are reverted, all the tokens resources remain in the storage.

const (
	tokenNameSize   = 16
	tokenKeySize    = 1 + tokenNameSize
	tokenRecordSize = 48
)

const (
	tokenKeyPrefix byte = iota
)

var (
	errInvalidDataSize = errors.New("invalid data size")
)

type tokenID [tokenNameSize]byte

type tokenKey struct {
	name tokenID
}

func (k *tokenKey) unmarshalBinary(data []byte) error {
	if len(data) != tokenKeySize {
		return errInvalidDataSize
	}
	copy(k.name[:], data[1:])
	return nil
}

func (k *tokenKey) marshalBinary() ([]byte, error) {
	keyBytes := make([]byte, tokenKeySize)
	keyBytes[0] = tokenKeyPrefix
	copy(keyBytes[1:], k.name[:])
	return keyBytes, nil
}

type tokenRecord struct {
	downloadBytes  int64
	uploadBytes    int64
	sectorAccesses int64
	keyValSets     int64
	keyValGets     int64
	keyValDeletes  int64
}

func (r *tokenRecord) unmarshalBinary(data []byte) error {
	if len(data) != tokenRecordSize {
		return errInvalidDataSize
	}
	r.downloadBytes = int64(binary.BigEndian.Uint64(data[:8]))
	r.uploadBytes = int64(binary.BigEndian.Uint64(data[8:16]))
	r.sectorAccesses = int64(binary.BigEndian.Uint64(data[16:24]))
	r.keyValSets = int64(binary.BigEndian.Uint64(data[24:32]))
	r.keyValGets = int64(binary.BigEndian.Uint64(data[32:40]))
	r.keyValDeletes = int64(binary.BigEndian.Uint64(data[40:48]))
	return nil
}

func (r *tokenRecord) marshalBinary() ([]byte, error) {
	buf := make([]byte, tokenRecordSize)
	binary.BigEndian.PutUint64(buf[:8], uint64(r.downloadBytes))
	binary.BigEndian.PutUint64(buf[8:16], uint64(r.uploadBytes))
	binary.BigEndian.PutUint64(buf[16:24], uint64(r.sectorAccesses))
	binary.BigEndian.PutUint64(buf[24:32], uint64(r.keyValSets))
	binary.BigEndian.PutUint64(buf[32:40], uint64(r.keyValGets))
	binary.BigEndian.PutUint64(buf[40:48], uint64(r.keyValDeletes))
	return buf, nil
}

type tokenStorage struct {
	db *leveldb.DB
}

func newTokenStorage(path string) (*tokenStorage, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, err
	}
	return &tokenStorage{db: db}, nil
}

func (s *tokenStorage) tokenRecord(id *tokenID) (*tokenRecord, error) {
	key := tokenKey{*id}
	keyBytes, err := key.marshalBinary()
	if err != nil {
		return nil, err
	}
	recordBytes, err := s.db.Get(keyBytes, nil)
	if err != nil {
		return nil, err
	}
	var record tokenRecord
	if err := record.unmarshalBinary(recordBytes); err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *tokenStorage) setTokenRecord(id *tokenID, r *tokenRecord) error {
	key := tokenKey{*id}
	keyBytes, err := key.marshalBinary()
	if err != nil {
		return err
	}
	recordBytes, err := r.marshalBinary()
	if err != nil {
		return err
	}
	if err := s.db.Put(keyBytes, recordBytes, nil); err != nil {
		return err
	}
	return nil
}

func (s *tokenStorage) addResources(id *tokenID, resourceType types.Specifier, amount int64) error {
	resources, err := s.tokenRecord(id)
	if err == leveldb.ErrNotFound {
		// New record.
		resources = &tokenRecord{}
	} else if err != nil {
		return err
	}
	switch resourceType {
	case modules.DownloadBytes:
		resources.downloadBytes += amount
	case modules.UploadBytes:
		resources.uploadBytes += amount
	case modules.SectorAccesses:
		resources.sectorAccesses += amount
	case modules.KeyValueSets:
		resources.keyValSets += amount
	case modules.KeyValueGets:
		resources.keyValGets += amount
	case modules.KeyValueDeletes:
		resources.keyValDeletes += amount
	}
	if err := s.setTokenRecord(id, resources); err != nil {
		return err
	}
	return nil
}

func (s *tokenStorage) close() error {
	return s.db.Close()
}
