package api

import (
	"fmt"

	"gitlab.com/scpcorp/ScPrime/crypto"
)

// DownloadWithTokenError represent error message
type DownloadWithTokenError struct {
	NotEnoughSectorAccesses bool         `json:"not_enough_sector_accesses,omitempty"`
	NotEnoughBytes          bool         `json:"not_enough_bytes,omitempty"`
	NoSuchSector            *crypto.Hash `json:"no_such_sector,omitempty"`
	UnknownError            string       `json:"unknown_error,omitempty"`
}

func (e DownloadWithTokenError) Error() string {
	return fmt.Sprintf("not enough sector accesses: %t \n"+
		"not enough bytes: %t \n"+
		"no such sector: [% x] \n"+
		"unknown error: %s", e.NotEnoughSectorAccesses, e.NotEnoughBytes, e.NoSuchSector, e.UnknownError)
}

// Range part of request
type Range struct {
	MerkleRoot  crypto.Hash `json:"merkle_root"`
	Offset      uint32      `json:"offset"`
	Length      uint32      `json:"length"`
	MerkleProof bool        `json:"merkle_proof"`
}

// DownloadWithTokenRequest represent request
type DownloadWithTokenRequest struct {
	TokenHex string  `header:"Authorization"`
	Ranges   []Range `json:"ranges"`
}

// Section part of response
type Section struct {
	Data        []byte        `json:"data"`
	MerkleProof []crypto.Hash `json:"merkle_proof"`
}

// DownloadWithTokenResponse represent response
type DownloadWithTokenResponse struct {
	Sections []Section `json:"sections"`
}
