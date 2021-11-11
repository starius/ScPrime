package host

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/host/api"
	"gitlab.com/scpcorp/ScPrime/types"
)

func TestAPI_DownloadWithToken(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	host, _ := blankMockHostTester(modules.ProdDependencies, t.Name())
	defer host.Close()
	hostApi := api.NewAPI(host.host.tokenStor, host.host.secretKey, host.host)

	// generate sector
	sectorData := fastrand.Bytes(int(modules.SectorSize))
	root := crypto.MerkleRoot(sectorData)

	// generate token
	b := fastrand.Bytes(16)
	var tokenID types.TokenID
	copy(tokenID[:], b)
	offset := 64
	length := 128

	req := &api.DownloadWithTokenRequest{
		Authorization: tokenID.String(),
		Ranges: []api.Range{{
			MerkleRoot:  root,
			MerkleProof: true,
			Length:      uint32(length),
			Offset:      uint32(offset),
		}},
	}

	// error not enough sector accesses, not enough bytes
	_, err := hostApi.DownloadWithToken(context.Background(), req)
	cErr := err.(*api.DownloadWithTokenError)
	if !cErr.NotEnoughSectorAccesses || !cErr.NotEnoughBytes {
		t.Fatal("should be errors: not enough sector accesses, not enough bytes")
	}

	// add DownloadBytes, error not enough sector accesses
	err = host.host.tokenStor.AddResources(tokenID, modules.DownloadBytes, 5000)
	_, err = hostApi.DownloadWithToken(context.Background(), req)
	cErr = err.(*api.DownloadWithTokenError)
	if !cErr.NotEnoughSectorAccesses {
		t.Fatal("should be error: not enough sector accesses")
	}

	// remove DownloadBytes, add SectorAccesses, error not enough bytes
	_ = host.host.tokenStor.RecordDownload(tokenID, 5000, 0)
	err = host.host.tokenStor.AddResources(tokenID, modules.SectorAccesses, 1)
	_, err = hostApi.DownloadWithToken(context.Background(), req)
	cErr = err.(*api.DownloadWithTokenError)
	if !cErr.NotEnoughBytes {
		t.Fatal("should be error: not enough bytes")
	}

	// error no such sector
	err = host.host.tokenStor.AddResources(tokenID, modules.DownloadBytes, 5000)
	_, err = hostApi.DownloadWithToken(context.Background(), req)
	cErr = err.(*api.DownloadWithTokenError)
	if cErr.NoSuchSector == nil {
		t.Fatal("should be error: no such sector")
	}

	// correct case
	err = host.host.tokenStor.AddResources(tokenID, modules.SectorAccesses, 1)
	err = host.host.tokenStor.AddResources(tokenID, modules.DownloadBytes, 5000)
	// create storage folder
	storageFolderOne := filepath.Join(host.host.persistDir, "hostTesterStorageFolderOne")
	err = os.Mkdir(storageFolderOne, 0700)
	if err != nil {
		t.Fatal("error creating storage folder")
	}
	err = host.host.AddStorageFolder(storageFolderOne, modules.SectorSize*64)
	if err != nil {
		t.Fatal("error adding storage folder")
	}
	err = host.host.StorageManager.AddSector(root, sectorData)
	if err != nil {
		t.Fatal("error adding sector")
	}
	resp, err := hostApi.DownloadWithToken(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Sections) != 1 {
		t.Fatal("incorrect resp data")
	}
	if !bytes.Equal(resp.Sections[0].Data, sectorData[offset:offset+length]) {
		t.Fatal("incorrect resp data")
	}
}

func TestAPI_UploadWithToken(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	host, _ := blankMockHostTester(modules.ProdDependencies, t.Name())
	defer host.Close()
	hostApi := api.NewAPI(host.host.tokenStor, host.host.secretKey, host.host)

	// generate token
	b := fastrand.Bytes(16)
	var tokenID types.TokenID
	copy(tokenID[:], b)

	// error empty sectors
	req := &api.UploadWithTokenRequest{
		Authorization: tokenID.String(),
		Sectors:       nil,
	}
	_, err := hostApi.UploadWithToken(context.Background(), req)
	cErr := err.(*api.UploadWithTokenError)
	if !cErr.DataLengthIsZero {
		t.Fatal("should be 'data length is zero' error")
	}

	// generate sector with incorrect size
	sectorData := fastrand.Bytes(10)
	req.Sectors = [][]byte{sectorData}
	_, err = hostApi.UploadWithToken(context.Background(), req)
	cErr = err.(*api.UploadWithTokenError)
	if !cErr.IncorrectSectorSize {
		t.Fatal("should be 'incorrect sector size' error")
	}

	//	generate 10 sectors with correct size
	req.Sectors = nil
	for i := 0; i < 10; i++ {
		req.Sectors = append(req.Sectors, fastrand.Bytes(int(modules.SectorSize)))
	}
	_, err = hostApi.UploadWithToken(context.Background(), req)
	cErr = err.(*api.UploadWithTokenError)
	if !cErr.NotEnoughBytes {
		t.Fatal("should be 'not enough bytes' error")
	}

	err = host.host.tokenStor.AddResources(tokenID, modules.UploadBytes, 41943041)
	_, err = hostApi.UploadWithToken(context.Background(), req)
	cErr = err.(*api.UploadWithTokenError)
	if !cErr.NotEnoughStorage {
		t.Fatal("should be 'not enough storage' error")
	}

	// correct case
	// add storage resource
	err = host.host.tokenStor.AddResources(tokenID, modules.Storage, 100)
	// create storage folder
	storageFolderOne := filepath.Join(host.host.persistDir, "hostTesterStorageFolderOne")
	err = os.Mkdir(storageFolderOne, 0700)
	if err != nil {
		t.Fatal("error creating storage folder")
	}
	err = host.host.AddStorageFolder(storageFolderOne, modules.SectorSize*64)
	if err != nil {
		t.Fatal("error adding storage folder")
	}
	_, err = hostApi.UploadWithToken(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAPI_CircleIntegration(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	rhp, err := newRenterHostPair(t.Name())
	defer rhp.Close()
	hostApi := api.NewAPI(rhp.staticHT.host.tokenStor, rhp.staticHT.host.secretKey, rhp.staticHT.host)

	// generate token
	b := fastrand.Bytes(16)
	var tokenID types.TokenID
	copy(tokenID[:], b)

	// add storage resource
	err = rhp.staticHT.host.tokenStor.AddResources(tokenID, modules.Storage, 1000)
	if err != nil {
		t.Fatal(err)
	}
	// add upload bytes resource
	err = rhp.staticHT.host.tokenStor.AddResources(tokenID, modules.UploadBytes, 41943041)
	if err != nil {
		t.Fatal(err)
	}
	// add download bytes resource
	err = rhp.staticHT.host.tokenStor.AddResources(tokenID, modules.DownloadBytes, 41943041)
	if err != nil {
		t.Fatal(err)
	}
	// add sector accesses bytes resource
	err = rhp.staticHT.host.tokenStor.AddResources(tokenID, modules.SectorAccesses, 10)
	if err != nil {
		t.Fatal(err)
	}

	// form upload with token request
	req := &api.UploadWithTokenRequest{
		Authorization: tokenID.String(),
	}
	req.Sectors = nil
	var sectorIDs []crypto.Hash
	for i := 0; i < 10; i++ {
		req.Sectors = append(req.Sectors, fastrand.Bytes(int(modules.SectorSize)))
		sectorIDs = append(sectorIDs, crypto.MerkleRoot(req.Sectors[i]))
	}

	// create storage folder
	storageFolderOne := filepath.Join(rhp.staticHT.host.persistDir, "hostTesterStorageFolderOne")
	err = os.Mkdir(storageFolderOne, 0700)
	if err != nil {
		t.Fatal("error creating storage folder")
	}
	err = rhp.staticHT.host.AddStorageFolder(storageFolderOne, modules.SectorSize*64)
	if err != nil {
		t.Fatal("error adding storage folder")
	}
	_, err = hostApi.UploadWithToken(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	//	attach to contract call
	so, err := rhp.staticHT.host.managedGetStorageObligation(rhp.staticFCID)
	if err != nil {
		t.Fatal(err)
	}
	currentRevision := so.RevisionTransactionSet[len(so.RevisionTransactionSet)-1].FileContractRevisions[0]
	revision := types.FileContractRevision{
		ParentID:              rhp.staticFCID,
		UnlockConditions:      currentRevision.UnlockConditions,
		NewRevisionNumber:     currentRevision.NewRevisionNumber + 1,
		NewFileSize:           modules.SectorSize * uint64(5), // move 5 sectors to contract
		NewFileMerkleRoot:     currentRevision.NewFileMerkleRoot,
		NewWindowStart:        currentRevision.NewWindowStart,
		NewWindowEnd:          currentRevision.NewWindowEnd,
		NewValidProofOutputs:  currentRevision.NewValidProofOutputs,
		NewMissedProofOutputs: currentRevision.NewMissedProofOutputs,
		NewUnlockHash:         currentRevision.NewUnlockHash,
	}
	sig := rhp.managedSign(revision)
	attachReq := &api.AttachSectorsRequest{
		ContractID: rhp.staticFCID,
		Sectors: []api.TokenAndSector{
			{
				Authorization: tokenID.String(),
				SectorID:      sectorIDs[0],
			},
			{
				Authorization: tokenID.String(),
				SectorID:      sectorIDs[3],
			},
			{
				Authorization: tokenID.String(),
				SectorID:      sectorIDs[5],
			},
			{
				Authorization: tokenID.String(),
				SectorID:      sectorIDs[7],
			},
			{
				Authorization: tokenID.String(),
				SectorID:      sectorIDs[9],
			},
		},
		Revision:        revision,
		RenterSignature: sig[:],
		BlockHeight:     rhp.staticHT.host.BlockHeight(),
	}
	_, err = hostApi.AttachSectors(context.Background(), attachReq)
	if err != nil {
		t.Fatal(err)
	}

	// Remove all sectors from temporary storage to check downloads from contract.
	removeReq := &api.RemoveSectorsRequest{
		Authorization: tokenID.String(),
		SectorIDs:     sectorIDs,
	}
	_, err = hostApi.RemoveSectors(context.Background(), removeReq)
	if err != nil {
		t.Fatal(err)
	}

	// download data
	offset := 64
	length := 128

	resp, err := hostApi.DownloadWithToken(context.Background(), &api.DownloadWithTokenRequest{
		Authorization: tokenID.String(),
		Ranges: []api.Range{
			// download 5 sectors which has moved from temporary storage to contract
			{
				MerkleRoot:  sectorIDs[0],
				MerkleProof: true,
				Length:      uint32(length),
				Offset:      uint32(offset),
			},
			{
				MerkleRoot:  sectorIDs[3],
				MerkleProof: true,
				Length:      uint32(length),
				Offset:      uint32(offset),
			},
			{
				MerkleRoot:  sectorIDs[5],
				MerkleProof: true,
				Length:      uint32(length),
				Offset:      uint32(offset),
			},
			{
				MerkleRoot:  sectorIDs[7],
				MerkleProof: true,
				Length:      uint32(length),
				Offset:      uint32(offset),
			},
			{
				MerkleRoot:  sectorIDs[9],
				MerkleProof: true,
				Length:      uint32(length),
				Offset:      uint32(offset),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// compare downloaded and uploaded data
	if !bytes.Equal(resp.Sections[0].Data, req.Sectors[0][offset:offset+length]) {
		t.Fatal("incorrect resp data")
	}
	if !bytes.Equal(resp.Sections[1].Data, req.Sectors[3][offset:offset+length]) {
		t.Fatal("incorrect resp data")
	}
	if !bytes.Equal(resp.Sections[2].Data, req.Sectors[5][offset:offset+length]) {
		t.Fatal("incorrect resp data")
	}
	if !bytes.Equal(resp.Sections[3].Data, req.Sectors[7][offset:offset+length]) {
		t.Fatal("incorrect resp data")
	}
	if !bytes.Equal(resp.Sections[4].Data, req.Sectors[9][offset:offset+length]) {
		t.Fatal("incorrect resp data")
	}
}
