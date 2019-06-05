package host

import (
	"bytes"
	"encoding/binary"

	"github.com/pkg/errors"
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

type tokenKey struct {
	name [tokenNameSize]byte
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
