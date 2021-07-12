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
	hostApi := api.NewAPI("", host.host.TokenStor, host.host.StorageManager, host.host.secretKey)

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
		TokenHex: tokenID.String(),
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
	err = host.host.TokenStor.AddResources(tokenID, modules.DownloadBytes, 5000)
	_, err = hostApi.DownloadWithToken(context.Background(), req)
	cErr = err.(*api.DownloadWithTokenError)
	if !cErr.NotEnoughSectorAccesses {
		t.Fatal("should be error: not enough sector accesses")
	}

	// remove DownloadBytes, add SectorAccesses, error not enough bytes
	_ = host.host.TokenStor.RecordDownload(tokenID, 5000, 0)
	err = host.host.TokenStor.AddResources(tokenID, modules.SectorAccesses, 1)
	_, err = hostApi.DownloadWithToken(context.Background(), req)
	cErr = err.(*api.DownloadWithTokenError)
	if !cErr.NotEnoughBytes {
		t.Fatal("should be error: not enough bytes")
	}

	// error no such sector
	err = host.host.TokenStor.AddResources(tokenID, modules.DownloadBytes, 5000)
	_, err = hostApi.DownloadWithToken(context.Background(), req)
	cErr = err.(*api.DownloadWithTokenError)
	if cErr.NoSuchSector == nil {
		t.Fatal("should be error: no such sector")
	}

	// correct case
	err = host.host.TokenStor.AddResources(tokenID, modules.SectorAccesses, 1)
	err = host.host.TokenStor.AddResources(tokenID, modules.DownloadBytes, 5000)
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
