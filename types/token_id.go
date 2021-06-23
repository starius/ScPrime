package types

import "fmt"

const (
	tokenNameSize = 16
)

// TokenID represent token type
type TokenID [tokenNameSize]byte

// String prints the id in hex.
func (t TokenID) String() string {
	return fmt.Sprintf("%x", t[:])
}
