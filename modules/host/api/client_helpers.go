package api

import (
	"context"
	"errors"
	"fmt"

	"gitlab.com/scpcorp/ScPrime/crypto"
)

// DownloadAndVerify calls Client.DownloadWithToken() and verifies merkle proofs.
func (c *Client) DownloadAndVerify(ctx context.Context, req *DownloadWithTokenRequest) (*DownloadWithTokenResponse, error) {
	resp, err := c.DownloadWithToken(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(req.Ranges) != len(resp.Sections) {
		return nil, fmt.Errorf("number of response sections %d does not match requested %d", len(resp.Sections), len(req.Ranges))
	}
	for i, reqRange := range req.Ranges {
		sec := resp.Sections[i]
		if len(sec.Data) != int(reqRange.Length) {
			return nil, errors.New("host did not send enough sector data")
		}
		if reqRange.MerkleProof {
			proofStart := int(reqRange.Offset) / crypto.SegmentSize
			proofEnd := int(reqRange.Offset+reqRange.Length) / crypto.SegmentSize
			if !crypto.VerifyRangeProof(sec.Data, sec.MerkleProof, proofStart, proofEnd, reqRange.MerkleRoot) {
				return nil, errors.New("host provided incorrect sector data or Merkle proof")
			}
		}
	}
	return resp, nil
}
