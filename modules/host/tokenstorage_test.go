package host

import (
	"io/ioutil"
	"math/rand"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createTokenStorage(t *testing.T) (*tokenStorage, string) {
	dbDir, err := ioutil.TempDir(os.TempDir(), "dbDir0")
	assert.NoError(t, err, "failed to create test data dir")
	stor, err := newTokenStorage(dbDir)
	assert.NoError(t, err, "newTokenStorage() failed")
	return stor, dbDir
}

func TestAddBytes(t *testing.T) {
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
	err = stor.addBytes(&id, amount)
	assert.NoError(t, err, "stor.addBytes() failed")
	savedAmount, err := stor.bytesAmount(&id)
	assert.NoError(t, err, "bytesAmount() failed")
	assert.Equal(t, amount, savedAmount, "bytes amount saved is not correct")
}

func TestAddSectors(t *testing.T) {
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
	err = stor.addSectors(&id, amount)
	assert.NoError(t, err, "stor.addSectors() failed")
	savedAmount, err := stor.sectorsAmount(&id)
	assert.NoError(t, err, "sectorsAmount() failed")
	assert.Equal(t, amount, savedAmount, "sectors amount saved is not correct")
}
