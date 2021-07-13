package api

import (
	"context"
	"errors"
	"math/bits"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
)

// DownloadWithToken handler for /download [POST] request
func (a *API) DownloadWithToken(ctx context.Context, req *DownloadWithTokenRequest) (*DownloadWithTokenResponse, error) {
	// Validate the request.
	if err := validateSections(req.Ranges); err != nil {
		return nil, &DownloadWithTokenError{UnknownError: err.Error()}
	}
	// Make sure token has enough resources to handle this call.
	id := types.ParseToken(req.TokenHex)
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
	if err := a.ts.RecordDownload(id, estBandwidth, sectorAccesses); err != nil {
		return nil, &DownloadWithTokenError{UnknownError: err.Error()}
	}
	var resp DownloadWithTokenResponse

	// Enter response loop.
	for _, sec := range req.Ranges {
		// Fetch the requested data.
		sectorData, err := a.sm.ReadSector(sec.MerkleRoot)
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
	return &resp, nil
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
		// proving across the two leaves in the center of the tree)
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
