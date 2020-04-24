package main

import (
	"strings"
	"testing"

	"gitlab.com/NebulousLabs/errors"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/node"
	"gitlab.com/scpcorp/ScPrime/pubaccesskey"
	"gitlab.com/scpcorp/ScPrime/siatest"
)

// TestSkykeyCommands tests the basic functionality of the siac pubaccesskey commands
// interface. More detailed testing of the pubaccesskey manager is done in the pubaccesskey
// package.
func TestSkykeyCommands(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create a node for the test
	n, err := siatest.NewNode(node.AllModules(build.TempDir(t.Name())))
	if err != nil {
		t.Fatal(err)
	}

	// Set global HTTP client to the node's client.
	httpClient = n.Client

	// Set the (global) cipher type to the only allowed type.
	// This is normally done by the flag parser.
	skykeyCipherType = "XChaCha20"

	testSkykeyString := "BAAAAAAAAABrZXkxAAAAAAAAAAQgAAAAAAAAADiObVg49-0juJ8udAx4qMW-TEHgDxfjA0fjJSNBuJ4a"
	err = skykeyAdd(testSkykeyString)
	if err != nil {
		t.Fatal(err)
	}

	err = skykeyAdd(testSkykeyString)
	if !strings.Contains(err.Error(), pubaccesskey.ErrSkykeyWithIDAlreadyExists.Error()) {
		t.Fatal("Unexpected duplicate name error", err)
	}

	// Change the key entropy, but keep the same name.
	var sk pubaccesskey.Pubaccesskey
	err = sk.FromString(testSkykeyString)
	if err != nil {
		t.Fatal(err)
	}
	sk.Entropy[0] ^= 1 // flip the first byte.

	skString, err := sk.ToString()
	if err != nil {
		t.Fatal(err)
	}

	// This should return a duplicate name error.
	err = skykeyAdd(skString)
	if !strings.Contains(err.Error(), pubaccesskey.ErrSkykeyWithNameAlreadyExists.Error()) {
		t.Fatal("Expected duplicate name error", err)
	}

	// Check that adding same key twice returns an error.
	keyName := "createkey1"
	newSkykey, err := skykeyCreate(keyName)
	if err != nil {
		t.Fatal(err)
	}
	_, err = skykeyCreate(keyName)
	if !strings.Contains(err.Error(), pubaccesskey.ErrSkykeyWithNameAlreadyExists.Error()) {
		t.Fatal("Expected error when creating key with same name")
	}

	// Check that invalid cipher types are caught.
	skykeyCipherType = "InvalidCipherType"
	_, err = skykeyCreate("createkey2")
	if !errors.Contains(err, crypto.ErrInvalidCipherType) {
		t.Fatal("Expected error when creating key with invalid ciphertype")
	}
	skykeyCipherType = "XChaCha20" //reset the ciphertype

	// Test skykeyGet
	// known key should have no errors.
	getKeyStr, err := skykeyGet(keyName, "")
	if err != nil {
		t.Fatal(err)
	}

	if getKeyStr != newSkykey {
		t.Fatal("Expected keys to match")
	}

	// Using both name and id params should return an error
	_, err = skykeyGet("name", "id")
	if err == nil {
		t.Fatal("Expected error when using both name and id")
	}
	// Using neither name or id param should return an error
	_, err = skykeyGet("", "")
	if err == nil {
		t.Fatal("Expected error when using neither name or id params")
	}
}
