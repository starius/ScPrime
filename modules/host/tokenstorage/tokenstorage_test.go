package tokenstorage

import (
	"context"
	"crypto/rand"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
)

func createTokenStorage(t *testing.T) (*TokenStorage, string) {
	dbDir, err := ioutil.TempDir(os.TempDir(), "dbDir0")
	assert.NoError(t, err, "failed to create test data dir")
	stor, err := NewTokenStorage(dbDir)
	assert.NoError(t, err, "NewTokenStorage() failed")
	return stor, dbDir
}

func TestAddResources(t *testing.T) {
	stor, path := createTokenStorage(t)

	defer func() {
		err := stor.Close(context.Background())
		assert.NoError(t, err, "failed to close tokenStorage")
		err = os.RemoveAll(path)
		assert.NoError(t, err, "failed to clean test data dir")
	}()

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
