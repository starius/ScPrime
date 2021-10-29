package tokenstorage

import (
	"context"
	"crypto/rand"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestAddResources(t *testing.T) {
	stor := createTokenStorage(t)
	amount := int64(100500)
	var id types.TokenID
	_, err := rand.Read(id[:])
	assert.NoError(t, err, "rand.Read() failed")
	err = stor.AddResources(id, modules.DownloadBytes, amount)
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
