package types

import (
	"encoding/hex"
	"fmt"
)

const (
	tokenNameSize = 16
)

// TokenID represent token type
type TokenID [tokenNameSize]byte

// String prints the id in hex.
func (t TokenID) String() string {
	return fmt.Sprintf("%x", t[:])
}

// ParseToken parse token from string
func ParseToken(t string) TokenID {
	tokenBytes, _ := hex.DecodeString(t)
	var token [16]byte
	copy(token[:], tokenBytes[:])
	return token
}

// Bytes convert TokenID to byte slice
func (t TokenID) Bytes() []byte {
	return t[:]
}
