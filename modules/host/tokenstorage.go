package host

import (
	"bytes"
	"encoding/binary"

	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	tokenNameSize   = 32
	tokenKeySize    = 1 + tokenNameSize
	tokenRecordSize = 8 + 8
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
	bytesAmount    int64
	sectorAccesses int64
}

func (r *tokenRecord) unmarshalBinary(data []byte) error {
	if len(data) != tokenRecordSize {
		return errInvalidDataSize
	}
	var err error
	r.bytesAmount, err = binary.ReadVarint(bytes.NewReader(data[:8]))
	if err != nil {
		return err
	}
	r.sectorAccesses, err = binary.ReadVarint(bytes.NewReader(data[8:16]))
	if err != nil {
		return err
	}
	return nil
}

func (r *tokenRecord) marshalBinary() ([]byte, error) {
	buf := make([]byte, tokenRecordSize)
	binary.PutVarint(buf[:8], r.bytesAmount)
	binary.PutVarint(buf[8:16], r.sectorAccesses)
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

func (s *tokenStorage) bytesAmount(id *tokenID) (int64, error) {
	record, err := s.tokenRecord(id)
	if err != nil {
		return 0, err
	}
	return record.bytesAmount, nil
}

func (s *tokenStorage) sectorsAmount(id *tokenID) (int64, error) {
	record, err := s.tokenRecord(id)
	if err != nil {
		return 0, err
	}
	return record.sectorAccesses, nil
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

func (s *tokenStorage) addBytes(id *tokenID, amount int64) error {
	record, err := s.tokenRecord(id)
	if err == leveldb.ErrNotFound {
		// Fresh record.
		record = &tokenRecord{}
	} else if err != nil {
		return err
	}
	record.bytesAmount += amount
	return s.setTokenRecord(id, record)
}

func (s *tokenStorage) addSectors(id *tokenID, amount int64) error {
	record, err := s.tokenRecord(id)
	if err == leveldb.ErrNotFound {
		// Fresh record.
		record = &tokenRecord{}
	} else if err != nil {
		return err
	}
	record.sectorAccesses += amount
	return s.setTokenRecord(id, record)
}

func (s *tokenStorage) close() error {
	return s.db.Close()
}
