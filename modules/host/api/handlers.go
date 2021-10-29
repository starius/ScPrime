package api

import (
	"context"
	"errors"
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
	enoughBytes := true
	enoughSectors := true
	availableBandwidth := int64(0)
	availableSectors := int64(0)
	tokenResources, err := a.ts.TokenRecord(id)
	if err == nil {
		// Token not found = no resources, and 0 is correct.
		availableBandwidth = tokenResources.DownloadBytes
		availableSectors = tokenResources.SectorAccesses
	}
	if availableBandwidth < estBandwidth {
		// Not enough download bandwidth.
		enoughBytes = false
	}

	if availableSectors < sectorAccesses {
		// Not enough sector accesses.
		enoughSectors = false
	}
	if !enoughBytes || !enoughSectors {
		// Send response indicating lack of resources.
		return nil, &DownloadWithTokenError{
			NotEnoughSectorAccesses: !enoughSectors,
			NotEnoughBytes:          !enoughBytes,
		}
	}
	if err = a.ts.RecordDownload(id, estBandwidth, sectorAccesses); err != nil {
		return nil, &DownloadWithTokenError{UnknownError: err.Error()}
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
	tokenResources, err = a.ts.TokenRecord(id)
	if err != nil {
		return nil, &DownloadWithTokenError{UnknownError: err.Error()}
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
	var sectorsIDs []crypto.Hash
	var totalBytes int64

	for _, sector := range req.Sectors {
		if uint64(len(sector)) != modules.SectorSize {
			return nil, &UploadWithTokenError{IncorrectSectorSize: true}
		}
		newRoot := crypto.MerkleRoot(sector)
		sectorsIDs = append(sectorsIDs, newRoot)
		totalBytes += int64(len(sector))
	}
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

	for _, sector := range req.Sectors {
		newRoot := crypto.MerkleRoot(sector)
		err = a.host.AddSector(newRoot, sector)
		if err != nil {
			return nil, &UploadWithTokenError{UnknownError: err.Error()}
		}
	}
	err = a.ts.AddSectors(id, sectorsIDs, time.Now())
	if err != nil {
		return nil, &UploadWithTokenError{UnknownError: err.Error()}
	}
	tr, err = a.ts.TokenRecord(id)
	if err != nil {
		return nil, &UploadWithTokenError{UnknownError: err.Error()}
	}
	// include updated information about token resources in response.
	return &UploadWithTokenResponse{TokenRecord: toTokenRecord(tr)}, nil
}

// AttachSectors handler for /attach [POST] request.
func (a *API) AttachSectors(ctx context.Context, req *AttachSectorsRequest) (*AttachSectorsResponse, error) {
	blockHeight := req.BlockHeight
	hostHeight := a.host.BlockHeight()
	if blockHeight != hostHeight && blockHeight != hostHeight-1 && blockHeight != hostHeight+1 {
		return nil, &AttachSectorsError{IncorrectBlock: true}
	}

	sectorIDs := make(map[types.TokenID][]crypto.Hash)
	for _, ts := range req.Sectors {
		tokenID := types.ParseToken(ts.Authorization)
		tokenSectors := sectorIDs[tokenID]
		tokenSectors = append(tokenSectors, ts.SectorID)
		sectorIDs[tokenID] = tokenSectors
	}

	hostSig, err := a.host.MoveTokenSectorsToStorageObligation(req.ContractID, req.Revision, sectorIDs, req.RenterSignature)
	if errors.Is(err, tokenstorage.ErrInsufficientResource) {
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
