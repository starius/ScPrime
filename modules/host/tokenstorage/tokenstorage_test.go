package tokenstorage

import (
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/host/contractmanager"
	"gitlab.com/scpcorp/ScPrime/types"
)

func createTokenStorage(t *testing.T) *TokenStorage {
	stDir, err := ioutil.TempDir(os.TempDir(), "stDir0")
	assert.NoError(t, err, "failed to create contract manager dir")
	t.Cleanup(func() {
		err = os.RemoveAll(stDir)
		assert.NoError(t, err, "failed to remove test dir")
	})
	stManager, err := contractmanager.NewCustomContractManager(new(modules.ProductionDependencies), stDir)
	assert.NoError(t, err, "NewCustomContractManager failed")
	t.Cleanup(func() {
		err = stManager.Close()
		assert.NoError(t, err, "failed to close contract manager")
	})
	dbDir, err := ioutil.TempDir(os.TempDir(), "dbDir0")
	assert.NoError(t, err, "failed to create test data dir")
	t.Cleanup(func() {
		err = os.RemoveAll(dbDir)
		assert.NoError(t, err, "failed to remove test dir")
	})
	stor, err := NewTokenStorage(stManager, dbDir)
	assert.NoError(t, err, "NewTokenStorage() failed")
	t.Cleanup(func() {
		err = stor.Close(context.Background())
		assert.NoError(t, err, "failed to close tokenStorage")
	})
	return stor
}

func createTokenStorageAndCheckExpiration(t *testing.T, period time.Duration) *TokenStorage {
	stor := createTokenStorage(t)
	done := make(chan bool)
	t.Cleanup(func() {
		done <- true
	})
	go stor.CheckExpiration(period, done)
	return stor
}

func TestAddResources(t *testing.T) {
	stor := createTokenStorage(t)
	amount := int64(100500)
	var id types.TokenID
	fastrand.Read(id[:])
	err := stor.AddResources(id, modules.DownloadBytes, amount)
	assert.NoError(t, err, "stor.addResources() failed")
	newResources, err := stor.TokenRecord(id)
	assert.NoError(t, err, "tokenRecord() failed")
	assert.Equal(t, amount, newResources.DownloadBytes)
	err = stor.AddResources(id, modules.UploadBytes, amount)
	assert.NoError(t, err, "stor.addResources() failed")
	newResources, err = stor.TokenRecord(id)
	assert.NoError(t, err, "tokenRecord() failed")
	assert.Equal(t, amount, newResources.UploadBytes)
}

func TestTokenStorage_AttachSectors(t *testing.T) {
	stor := createTokenStorageAndCheckExpiration(t, time.Second)
	var token types.TokenID
	fastrand.Read(token[:])
	sector := fastrand.Bytes(int(modules.SectorSize))
	sector1 := fastrand.Bytes(int(modules.SectorSize))
	sectorID := crypto.MerkleRoot(sector)
	sectorID1 := crypto.MerkleRoot(sector1)
	sectorsAmount := int64(2)
	storageTimeSeconds := int64(10)
	uploadBytesAmount := int64(modules.SectorSize) * sectorsAmount
	storageAmount := storageTimeSeconds * sectorsAmount
	assert.NoError(t, stor.AddResources(token, modules.UploadBytes, uploadBytesAmount))
	assert.NoError(t, stor.AddResources(token, modules.Storage, storageAmount))
	additionTime := time.Now()
	assert.NoError(t, stor.AddSectors(token, []crypto.Hash{sectorID, sectorID1}, additionTime))
	tr, err := stor.TokenRecord(token)
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), tr.TokenStorageInfo.SectorsNum)
	attachSectors := map[types.TokenID][]crypto.Hash{token: {sectorID, sectorID1}}
	assert.NoError(t, stor.AttachSectors(attachSectors, time.Now()))
	tr, err = stor.TokenRecord(token)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), tr.TokenStorageInfo.SectorsNum)
}

func TestTokenStorage_RemoveUnpaidSectors(t *testing.T) {
	expirationCheckPeriod := 7 * time.Second
	stor := createTokenStorageAndCheckExpiration(t, expirationCheckPeriod)
	var token types.TokenID
	fastrand.Read(token[:])
	sector := fastrand.Bytes(int(modules.SectorSize))
	sector1 := fastrand.Bytes(int(modules.SectorSize))
	sectorID := crypto.MerkleRoot(sector)
	sectorID1 := crypto.MerkleRoot(sector1)
	sectorsAmount := int64(2)
	storageTimeSeconds := int64(5)
	uploadBytesAmount := int64(modules.SectorSize) * sectorsAmount
	storageAmount := storageTimeSeconds * sectorsAmount
	assert.NoError(t, stor.AddResources(token, modules.UploadBytes, uploadBytesAmount))
	assert.NoError(t, stor.AddResources(token, modules.Storage, storageAmount))
	additionTime := time.Now()
	assert.NoError(t, stor.AddSectors(token, []crypto.Hash{sectorID, sectorID1}, additionTime))
	tr, err := stor.TokenRecord(token)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), tr.DownloadBytes)
	assert.Equal(t, int64(0), tr.UploadBytes)
	assert.Equal(t, int64(0), tr.SectorAccesses)
	assert.Equal(t, uint64(2), tr.TokenStorageInfo.SectorsNum)
	assert.True(t, tr.TokenStorageInfo.Storage > 0 && tr.TokenStorageInfo.Storage <= storageAmount)
	assert.True(t, tr.TokenStorageInfo.LastChangeTime.Equal(additionTime))
	sectorIDs, _, err := stor.ListSectorIDs(token, "", 10)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(sectorIDs))
	assert.ElementsMatch(t, []crypto.Hash{sectorID, sectorID1}, sectorIDs)
	enough, err := stor.EnoughStorageResource(token, 0, time.Now())
	assert.NoError(t, err)
	assert.True(t, enough)

	beforeSleep := time.Now()
	time.Sleep(time.Duration(storageTimeSeconds)*time.Second + expirationCheckPeriod)
	afterSleep := time.Now()

	sectorIDs, _, err = stor.ListSectorIDs(token, "", 1)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(sectorIDs))
	tr, err = stor.TokenRecord(token)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), tr.DownloadBytes)
	assert.Equal(t, int64(0), tr.UploadBytes)
	assert.Equal(t, int64(0), tr.SectorAccesses)
	assert.Equal(t, uint64(0), tr.TokenStorageInfo.SectorsNum)
	assert.Equal(t, int64(0), tr.TokenStorageInfo.Storage)
	assert.True(t, tr.TokenStorageInfo.LastChangeTime.After(beforeSleep))
	assert.True(t, tr.TokenStorageInfo.LastChangeTime.Before(afterSleep))
	enough, err = stor.EnoughStorageResource(token, 0, time.Now())
	assert.NoError(t, err)
	assert.False(t, enough)
}
