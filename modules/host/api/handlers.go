package api

import (
	"context"
	"errors"
	"log"
	"math/bits"
	"time"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/host/tokenstorage"
	"gitlab.com/scpcorp/ScPrime/types"
)

// DefaultListSectorIDsLimit limits range of results for sectorIDs listing.
const DefaultListSectorIDsLimit = 10000

// ListSectorIDs handler for /list-sector-ids [GET] request.
func (a *API) ListSectorIDs(ctx context.Context, req *ListSectorIDsRequest) (*ListSectorIDsResponse, error) {
	id := types.ParseToken(req.Authorization)
	sectorIDs, nextPageID, err := a.ts.ListSectorIDs(id, req.PageID, DefaultListSectorIDsLimit)
	if err != nil {
		return nil, err
	}

	return &ListSectorIDsResponse{
		SectorIDs:  sectorIDs,
		NextPageID: nextPageID,
	}, nil
}

// RemoveSectors handler for /remove-sectors [POST] request.
func (a *API) RemoveSectors(ctx context.Context, req *RemoveSectorsRequest) (*RemoveSectorsResponse, error) {
	id := types.ParseToken(req.Authorization)
	if err := a.ts.RemoveSpecificSectors(id, req.SectorIDs, time.Now()); err != nil {
		return nil, err
	}

	return &RemoveSectorsResponse{}, nil
}

// TokenResources handler for /resources [GET] request.
func (a *API) TokenResources(ctx context.Context, req *TokenResourcesRequest) (*TokenResourcesResponse, error) {
	id := types.ParseToken(req.Authorization)
	tr, err := a.ts.TokenRecord(id)
	if err != nil {
		return nil, err
	}

	return &TokenResourcesResponse{
		UploadBytes:    tr.UploadBytes,
		DownloadBytes:  tr.DownloadBytes,
		SectorAccesses: tr.SectorAccesses,
		Storage:        tr.TokenStorageInfo.Storage,
		LastChangeTime: tr.TokenStorageInfo.LastChangeTime,
	}, nil
}

// DownloadWithToken handler for /download [POST] request.
func (a *API) DownloadWithToken(ctx context.Context, req *DownloadWithTokenRequest) (*DownloadWithTokenResponse, error) {
	// Validate the request.
	if err := validateSections(req.Ranges); err != nil {
		return nil, &DownloadWithTokenError{UnknownError: err.Error()}
	}
	// Make sure token has enough resources to handle this call.
	id := types.ParseToken(req.Authorization)
	estBandwidth := estimateBandwidth(req.Ranges)
	sectorAccesses := estimateSectorsAccesses(req.Ranges)
	tokenResources, err := a.ts.RecordDownload(id, estBandwidth, sectorAccesses, time.Now())
	if err != nil {
		var downloadWithTokenErr DownloadWithTokenError
		if errors.Is(err, tokenstorage.ErrInsufficientDownloadBytesAndSectorAccesses) {
			downloadWithTokenErr.NotEnoughSectorAccesses = true
			downloadWithTokenErr.NotEnoughBytes = true
		} else if errors.Is(err, tokenstorage.ErrInsufficientDownloadBytes) {
			downloadWithTokenErr.NotEnoughBytes = true
		} else if errors.Is(err, tokenstorage.ErrInsufficientSectorAccesses) {
			downloadWithTokenErr.NotEnoughSectorAccesses = true
		} else {
			downloadWithTokenErr.UnknownError = err.Error()
		}
		return nil, &downloadWithTokenErr
	}

	var resp DownloadWithTokenResponse
	// Enter response loop.
	for _, sec := range req.Ranges {
		// Fetch the requested data.
		sectorData, err := a.host.ReadSector(sec.MerkleRoot)
		if err != nil {
			return nil, &DownloadWithTokenError{NoSuchSector: &sec.MerkleRoot}
		}
		data := sectorData[sec.Offset : sec.Offset+sec.Length]

		// Construct the Merkle proof, if requested.
		var proof []crypto.Hash
		if sec.MerkleProof {
			proofStart := int(sec.Offset) / crypto.SegmentSize
			proofEnd := int(sec.Offset+sec.Length) / crypto.SegmentSize
			proof = crypto.MerkleRangeProof(sectorData, proofStart, proofEnd)
		}
		resp.Sections = append(resp.Sections, Section{
			Data:        data,
			MerkleProof: proof,
		})
	}
	// include updated information about token resources in response.
	resp.TokenRecord = toTokenRecord(tokenResources)
	return &resp, nil
}

// UploadWithToken handler for /upload [POST] request.
func (a *API) UploadWithToken(ctx context.Context, req *UploadWithTokenRequest) (*UploadWithTokenResponse, error) {
	if len(req.Sectors) == 0 {
		return nil, &UploadWithTokenError{DataLengthIsZero: true}
	}
	id := types.ParseToken(req.Authorization)
	tr, err := a.ts.TokenRecord(id)
	if err != nil {
		return nil, &UploadWithTokenError{UnknownError: err.Error()}
	}
	var totalBytes int64

	sectorsByIDs := make(map[crypto.Hash][]byte, len(req.Sectors))
	for _, sector := range req.Sectors {
		if uint64(len(sector)) != modules.SectorSize {
			return nil, &UploadWithTokenError{IncorrectSectorSize: true}
		}
		newRoot := crypto.MerkleRoot(sector)
		sectorsByIDs[newRoot] = sector
	}

	sectorsIDs := make([]crypto.Hash, 0, len(sectorsByIDs))
	for sectorID, sector := range sectorsByIDs {
		sectorsIDs = append(sectorsIDs, sectorID)
		totalBytes += int64(len(sector))
	}

	// tokenStorage.AddSectors checks resources again under stateMu to make sure
	// we don't run into negative amounts due to a race (OK here, decreased in other goroutine before
	// we reach tokenSectors.AddSectors). However, we still need to check it here
	// to prevent attacking the host with empty tokens: such an attack requires no money,
	// but makes host write sectors to disk, since tokenSectors.AddSectors is called after.
	if totalBytes > tr.UploadBytes {
		return nil, &UploadWithTokenError{NotEnoughBytes: true, TokenRecord: toTokenRecord(tr)}
	}
	enoughResource, err := a.ts.EnoughStorageResource(id, int64(len(sectorsIDs)), time.Now())
	if err != nil {
		return nil, &UploadWithTokenError{UnknownError: err.Error()}
	}
	if !enoughResource {
		return nil, &UploadWithTokenError{NotEnoughStorage: true, TokenRecord: toTokenRecord(tr)}
	}

	for sectorID, sector := range sectorsByIDs {
		if err := a.host.AddSector(sectorID, sector); err != nil {
			return nil, &UploadWithTokenError{UnknownError: err.Error()}
		}
	}
	tr, err = a.ts.AddSectors(id, sectorsIDs, time.Now())
	if err != nil {
		// If it fails, remove sectors from StorageManager.
		go func() {
			if err := a.host.RemoveSectorBatch(sectorsIDs); err != nil {
				log.Printf("Failed to remove sectors after failed TokenStorage.AddSectors: %v", err)
			}
		}()
		if errors.Is(err, tokenstorage.ErrInsufficientUploadBytes) {
			return nil, &UploadWithTokenError{NotEnoughBytes: true, TokenRecord: toTokenRecord(tr)}
		} else if errors.Is(err, tokenstorage.ErrInsufficientStorage) {
			return nil, &UploadWithTokenError{NotEnoughStorage: true, TokenRecord: toTokenRecord(tr)}
		}
		return nil, &UploadWithTokenError{UnknownError: err.Error(), TokenRecord: toTokenRecord(tr)}
	}
	// Include updated information about token resources in response.
	return &UploadWithTokenResponse{TokenRecord: toTokenRecord(tr)}, nil
}

// AttachSectors handler for /attach [POST] request.
func (a *API) AttachSectors(ctx context.Context, req *AttachSectorsRequest) (*AttachSectorsResponse, error) {
	blockHeight := req.BlockHeight
	hostHeight := a.host.BlockHeight()
	if blockHeight != hostHeight && blockHeight != hostHeight-1 && blockHeight != hostHeight+1 {
		return nil, &AttachSectorsError{IncorrectBlock: true}
	}

	sectorsWithTokens := make([]types.SectorWithToken, 0, len(req.Sectors))
	for _, ts := range req.Sectors {
		tokenID := types.ParseToken(ts.Authorization)
		sectorsWithTokens = append(sectorsWithTokens, types.SectorWithToken{SectorID: ts.SectorID, Token: tokenID})
	}

	hostSig, err := a.host.MoveTokenSectorsToStorageObligation(req.ContractID, req.Revision, sectorsWithTokens, req.RenterSignature)
	if errors.Is(err, tokenstorage.ErrInsufficientStorage) {
		return nil, &AttachSectorsError{NotEnoughStorage: true}
	} else if err != nil {
		return nil, &AttachSectorsError{UnknownError: err.Error()}
	}
	return &AttachSectorsResponse{HostSignature: hostSig}, nil
}

func validateSections(sections []Range) error {
	for _, section := range sections {
		var err error
		switch {
		case uint64(section.Offset)+uint64(section.Length) > modules.SectorSize:
			err = errors.New("download request has invalid sector bounds")
		case section.Length == 0:
			err = errors.New("length cannot be zero")
		case section.MerkleProof && (section.Offset%crypto.SegmentSize != 0 || section.Length%crypto.SegmentSize != 0):
			err = errors.New("offset and length must be multiples of SegmentSize when requesting a Merkle proof")
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func estimateBandwidth(sections []Range) int64 {
	var estBandwidth int64
	for _, section := range sections {
		// use the worst-case proof size of 2*tree depth (this occurs when
		// proving across the two leaves in the center of the tree).
		estHashesPerProof := 2 * bits.Len64(modules.SectorSize/crypto.SegmentSize)
		estBandwidth += int64(section.Length) + int64(estHashesPerProof*crypto.HashSize)
	}
	if estBandwidth < modules.RPCMinLen {
		estBandwidth = modules.RPCMinLen
	}
	return estBandwidth
}

func estimateSectorsAccesses(sections []Range) int64 {
	sectorAccesses := make(map[crypto.Hash]struct{})
	for _, sec := range sections {
		sectorAccesses[sec.MerkleRoot] = struct{}{}
	}
	return int64(len(sectorAccesses))
}
