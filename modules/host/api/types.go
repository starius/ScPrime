package api

import (
	"bytes"
	"fmt"
	"text/template"

	"gitlab.com/scpcorp/ScPrime/modules/host/tokenstorage"

	"gitlab.com/scpcorp/ScPrime/crypto"
)

// TokenStorageInfo represent info about token storage resource
type TokenStorageInfo struct {
	Storage    uint64 `json:"storage"` // sectors * second
	SectorsNum uint64 `json:"sectors_num"`
}

// TokenRecord include information about token record
type TokenRecord struct {
	DownloadBytes  int64            `json:"download_bytes"`
	UploadBytes    int64            `json:"upload_bytes"`
	SectorAccesses int64            `json:"sector_accesses"`
	TokenInfo      TokenStorageInfo `json:"token_info"`
}

func toTokenRecord(record tokenstorage.TokenRecord) *TokenRecord {
	return &TokenRecord{
		DownloadBytes:  record.DownloadBytes,
		UploadBytes:    record.UploadBytes,
		SectorAccesses: record.SectorAccesses,
		TokenInfo: TokenStorageInfo{
			Storage:    record.TokenStorageInfo.Storage,
			SectorsNum: record.TokenStorageInfo.SectorsNum,
		},
	}
}

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

// Authorization represent request authorization
type Authorization struct {
	HostToken string `json:"token_hex"`
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
	Authorization Authorization `header:"Authorization"`
	Ranges        []Range       `json:"ranges"`
}

// Section part of response
type Section struct {
	Data        []byte        `json:"data"`
	MerkleProof []crypto.Hash `json:"merkle_proof"`
}

// DownloadWithTokenResponse represent response
type DownloadWithTokenResponse struct {
	Sections    []Section    `json:"sections"`
	TokenRecord *TokenRecord `json:"token_record,omitempty"`
}

// UploadWithTokenError represent error message
type UploadWithTokenError struct {
	DataLengthIsZero    bool         `json:"data_length_is_zero,omitempty"`
	IncorrectSectorSize bool         `json:"incorrect_sector_size,omitempty"`
	NotEnoughBytes      bool         `json:"not_enough_bytes,omitempty"`
	NotEnoughStorage    bool         `json:"not_enough_storage,omitempty"`
	UnknownError        string       `json:"unknown_error,omitempty"`
	TokenRecord         *TokenRecord `json:"token_record,omitempty"`
}

var uploadWithTokenErrorTemplate = template.Must(template.New("error").Parse(`
incorrect sector size: {{.IncorrectSectorSize}}
not enough bytes: {{.NotEnoughBytes}}
not enough storage: {{.NotEnoughStorage}}
unknown error: {{.UnknownError}}
Token Record:
download bytes: {{.TokenRecord.DownloadBytes}}
upload bytes: {{.TokenRecord.UploadBytes}}
sector accesses: {{.TokenRecord.SectorAccesses}}
storage: {{.TokenRecord.TokenInfo.Storage}}
sectors num: {{.TokenRecord.TokenInfo.SectorsNum}}
`))

func (e UploadWithTokenError) Error() string {
	var tpl bytes.Buffer
	_ = uploadWithTokenErrorTemplate.Execute(&tpl, e)
	return tpl.String()
}

// UploadWithTokenRequest represent request data
type UploadWithTokenRequest struct {
	Authorization Authorization `header:"Authorization"`
	Sectors       [][]byte      `json:"sectors"`
}

// UploadWithTokenResponse represent response data
type UploadWithTokenResponse struct {
	TokenRecord *TokenRecord `json:"token_record,omitempty"`
}
