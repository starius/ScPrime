package build

import (
	"os"
	"testing"
)

// TestAPIPassword tests getting and setting the API Password
func TestAPIPassword(t *testing.T) {
	// Unset any defaults, this only affects in memory state. Any Env Vars will
	// remain intact on disk
	err := os.Unsetenv(EnvvarAPIPassword)
	if err != nil {
		t.Error(err)
	}

	// Calling APIPassword should return a non-blank password if the env
	// variable isn't set
	pw, err := APIPassword()
	if err != nil {
		t.Error(err)
	}
	if pw == "" {
		t.Error("Password should not be blank")
	}

	// Test setting the env variable
	newPW := "abc123"
	err = os.Setenv(EnvvarAPIPassword, newPW)
	if err != nil {
		t.Error(err)
	}
	pw, err = APIPassword()
	if err != nil {
		t.Error(err)
	}
	if pw != newPW {
		t.Errorf("Expected password to be %v but was %v", newPW, pw)
	}
}

// TestSiadDataDir tests getting and setting the Sia consensus directory
func TestSiadDataDir(t *testing.T) {
	t.Skipf("Env var %v is unused", EnvvarDaemonDataDir)
	// Unset any defaults, this only affects in memory state. Any Env Vars will
	// remain intact on disk
	err := os.Unsetenv(EnvvarDaemonDataDir)
	if err != nil {
		t.Error(err)
	}

	// Test Default SiadDataDir
	siadDir := SiadDataDir()
	if siadDir == "" {
		t.Errorf("Expected siadDir to be pointing to %v but was empty", siadDir)
	}

	// Test Env Variable
	newSiaDir := "foo/bar"
	err = os.Setenv(EnvvarDaemonDataDir, newSiaDir)
	if err != nil {
		t.Error(err)
	}
	siadDir = SiadDataDir()
	if siadDir != newSiaDir {
		t.Errorf("Expected siadDir to be %v but was %v", newSiaDir, siadDir)
	}
}

// TestSiaDir tests getting and setting the Sia data directory
func TestSiaDir(t *testing.T) {
	// Unset any defaults, this only affects in memory state. Any Env Vars will
	// remain intact on disk
	err := os.Unsetenv(EnvvarMetaDataDir)
	if err != nil {
		t.Error(err)
	}

	// Test Default SiaDir
	siaDir := SiaDir()
	if siaDir != DefaultMetadataDir() {
		t.Errorf("Expected siaDir to be %v but was %v", DefaultMetadataDir(), siaDir)
	}

	// Test Env Variable
	newSiaDir := "foo/bar"
	err = os.Setenv(EnvvarMetaDataDir, newSiaDir)
	if err != nil {
		t.Error(err)
	}
	siaDir = SiaDir()
	if siaDir != newSiaDir {
		t.Errorf("Expected siaDir to be %v but was %v", newSiaDir, siaDir)
	}
}

// TestSiaWalletPassword tests getting and setting the Sia Wallet Password
func TestSiaWalletPassword(t *testing.T) {
	// Unset any defaults, this only affects in memory state. Any Env Vars will
	// remain intact on disk
	err := os.Unsetenv(EnvvarWalletPassword)
	if err != nil {
		t.Error(err)
	}

	// Test Default Wallet Password
	pw := WalletPassword()
	if pw != "" {
		t.Errorf("Expected wallet password to be blank but was %v", pw)
	}

	// Test Env Variable
	newPW := "abc123"
	err = os.Setenv(EnvvarWalletPassword, newPW)
	if err != nil {
		t.Error(err)
	}
	pw = WalletPassword()
	if pw != newPW {
		t.Errorf("Expected wallet password to be %v but was %v", newPW, pw)
	}
}
