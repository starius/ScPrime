package host

import (
	"crypto/rand"
	"io/ioutil"
	"os"
	"testing"

	"gitlab.com/scpcorp/ScPrime/modules"

	"github.com/stretchr/testify/assert"
)

func createTokenStorage(t *testing.T) (*tokenStorage, string) {
	dbDir, err := ioutil.TempDir(os.TempDir(), "dbDir0")
	assert.NoError(t, err, "failed to create test data dir")
	stor, err := newTokenStorage(dbDir)
	assert.NoError(t, err, "newTokenStorage() failed")
	return stor, dbDir
}

func TestAddResources(t *testing.T) {
	stor, path := createTokenStorage(t)

	defer func() {
		err := os.RemoveAll(path)
		assert.NoError(t, err, "failed to clean test data dir")
		err = stor.close()
		assert.NoError(t, err, "failed to close tokenStorage")
	}()

	amount := int64(100500)
	var id tokenID
	_, err := rand.Read(id[:])
	assert.NoError(t, err, "rand.Read() failed")
	err = stor.addResources(&id, modules.DownloadBytes, amount)
	assert.NoError(t, err, "stor.addResources() failed")
	newResources, err := stor.tokenRecord(&id)
	assert.NoError(t, err, "tokenRecord() failed")
	assert.Equal(t, amount, newResources.downloadBytes)
	err = stor.addResources(&id, modules.UploadBytes, amount)
	assert.NoError(t, err, "stor.addResources() failed")
	newResources, err = stor.tokenRecord(&id)
	assert.NoError(t, err, "tokenRecord() failed")
	assert.Equal(t, amount, newResources.uploadBytes)
}
