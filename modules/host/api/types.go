package api

import (
	"bytes"
	"text/template"
	"time"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules/host/tokenstorage"
	"gitlab.com/scpcorp/ScPrime/types"
)

//go:generate go run ./gen/...

// TokenStorageInfo represent info about token storage resource.
type TokenStorageInfo struct {
	Storage        int64     `json:"storage"` // sectors * second.
	SectorsNum     uint64    `json:"sectors_num"`
	LastChangeTime time.Time `json:"last_change_time"`
}

// TokenRecord include information about token record.
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
			Storage:        record.TokenStorageInfo.Storage,
			SectorsNum:     record.TokenStorageInfo.SectorsNum,
			LastChangeTime: record.TokenStorageInfo.LastChangeTime,
		},
	}
}

// ListSectorIDsRequest represents request.
type ListSectorIDsRequest struct {
	Authorization string `header:"Authorization"`
	PageID        string `json:"page_id"`
}

// ListSectorIDsResponse represents response.
type ListSectorIDsResponse struct {
	SectorIDs  []crypto.Hash `json:"sector_ids"`
	NextPageID string        `json:"next_page_id"`
}

// RemoveSectorsRequest represents request.
type RemoveSectorsRequest struct {
	Authorization string `header:"Authorization"`
	SectorIDs     []crypto.Hash
}

// RemoveSectorsResponse represents response.
type RemoveSectorsResponse struct{}

// TokenResourcesRequest represents request.
type TokenResourcesRequest struct {
	Authorization string `header:"Authorization"`
}

// TokenResourcesResponse represents response.
type TokenResourcesResponse struct {
	UploadBytes    int64     `json:"upload_bytes,omitempty"`
	DownloadBytes  int64     `json:"download_bytes,omitempty"`
	SectorAccesses int64     `json:"sector_accesses,omitempty"`
	Storage        int64     `json:"storage,omitempty"`
	LastChangeTime time.Time `json:"last_change_time,omitempty"`
}

// DownloadWithTokenError represent error message.
type DownloadWithTokenError struct {
	NotEnoughSectorAccesses bool         `json:"not_enough_sector_accesses,omitempty"`
	NotEnoughBytes          bool         `json:"not_enough_bytes,omitempty"`
	NoSuchSector            *crypto.Hash `json:"no_such_sector,omitempty"`
	UnknownError            string       `json:"unknown_error,omitempty"`
}

var downloadWithTokenErrorTemplate = template.Must(template.New("error").Parse(`
not enough sector accesses: {{.NotEnoughSectorAccesses}}
not enough bytes: {{.NotEnoughBytes}}
no such sector: {{.NoSuchSector}}
unknown error: {{.UnknownError}}
`))

func (e DownloadWithTokenError) Error() string {
	var tpl bytes.Buffer
	_ = downloadWithTokenErrorTemplate.Execute(&tpl, e)
	return tpl.String()
}

// Range part of request.
type Range struct {
	MerkleRoot  crypto.Hash `json:"merkle_root"`
	Offset      uint32      `json:"offset"`
	Length      uint32      `json:"length"`
	MerkleProof bool        `json:"merkle_proof"`
}

// DownloadWithTokenRequest represent request.
type DownloadWithTokenRequest struct {
	Authorization string  `header:"Authorization"`
	Ranges        []Range `json:"ranges"`
}

// Section part of response.
type Section struct {
	Data        []byte        `json:"data"`
	MerkleProof []crypto.Hash `json:"merkle_proof"`
}

// DownloadWithTokenResponse represent response.
type DownloadWithTokenResponse struct {
	Sections    []Section    `json:"sections"`
	TokenRecord *TokenRecord `json:"token_record,omitempty"`
}

// UploadWithTokenError represent error message.
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

// UploadWithTokenRequest represent request data.
type UploadWithTokenRequest struct {
	Authorization string   `header:"Authorization"`
	Sectors       [][]byte `json:"sectors"`
}

// UploadWithTokenResponse represent response data.
type UploadWithTokenResponse struct {
	TokenRecord *TokenRecord `json:"token_record,omitempty"`
}

// TokenAndSector include information about token and sectors.
type TokenAndSector struct {
	Authorization string      `json:"Authorization"`
	SectorID      crypto.Hash `json:"sector_id"`

	// TODO: `KeepInTmp` is commented, because there is no way to implement
	// it correctly with current architecture. It creates two ways of payment
	// for the same sector after AttachSetor is called or requires copying.

	// If true. keep the sector in the temporary store.
	// If false, the sector is moved from temporary store to the contract.
	// KeepInTmp bool `json:"keep_in_tmp"`
}

// AttachSectorsRequest represent request data.
type AttachSectorsRequest struct {
	ContractID      types.FileContractID       `json:"contract_id"`
	Sectors         []TokenAndSector           `json:"sectors"`
	Revision        types.FileContractRevision `json:"revision"`
	RenterSignature []byte                     `json:"renter_signature"`
	BlockHeight     types.BlockHeight          `json:"block_height"` // must be current or previous block
}

// AttachSectorsResponse represent response data.
type AttachSectorsResponse struct {
	HostSignature []byte `json:"host_signature"`
}

// AttachSectorsError represent error message.
type AttachSectorsError struct {
	IncorrectBlock   bool   `json:"incorrect_block,omitempty"`
	NotEnoughStorage bool   `json:"not_enough_storage,omitempty"`
	UnknownError     string `json:"unknown_error,omitempty"`
}

var attachSectorsErrorTemplate = template.Must(template.New("error").Parse(`
incorrect block: {{.IncorrectBlock}}
not enough storage: {{.NotEnoughStorage}}
unknown error: {{.UnknownError}}
`))

func (e AttachSectorsError) Error() string {
	var tpl bytes.Buffer
	_ = attachSectorsErrorTemplate.Execute(&tpl, e)
	return tpl.String()
}
