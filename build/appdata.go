package build

import (
	"encoding/hex"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gitlab.com/NebulousLabs/fastrand"
)

const (
	// EnvvarAPIPassword is the environment variable that sets a custom API
	// password if the default is not used
	EnvvarAPIPassword = "SCPRIME_API_PASSWORD"

	// EnvvarMetaDataDir is the environment variable that tells spd where to put the
	// sia data
	EnvvarMetaDataDir = "SCPRIME_DATA_DIR"

	// EnvvarWalletPassword is the environment variable that can be set to enable
	// auto unlocking the wallet
	EnvvarWalletPassword = "SCPRIME_WALLET_PASSWORD"
)

// APIPassword returns the Sia API Password either from the environment variable
// or from the password file. If no environment variable is set and no file
// exists, a password file is created and that password is returned
func APIPassword() (string, error) {
	// Check the environment variable.
	pw := os.Getenv(EnvvarAPIPassword)
	if pw != "" {
		return pw, nil
	}

	// Try to read the password from disk.
	path := apiPasswordFilePath()
	pwFile, err := ioutil.ReadFile(path)
	if err == nil {
		// This is the "normal" case, so don't print anything.
		return strings.TrimSpace(string(pwFile)), nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	// No password file; generate a secure one.
	// Generate a password file.
	pw, err = createAPIPasswordFile()
	if err != nil {
		return "", err
	}
	return pw, nil
}

// SiadDataDir returns the siad consensus data directory from the
// environment variable. If there is no environment variable it returns an empty
// string, instructing siad to store the consensus in the current directory.
func SiadDataDir() string {
	return os.Getenv(siadDataDir)
}

// SiaDir returns the Sia data directory either from the environment variable or
// the default.
func SiaDir() string {
	dataDir := os.Getenv(EnvvarMetaDataDir)
	if dataDir == "" {
		dataDir = DefaultMetadataDir()
	}
	return dataDir
}

// SkynetDir returns the Skynet data directory.
func SkynetDir() string {
	return defaultSkynetDir()
}

// WalletPassword returns the SiaWalletPassword environment variable.
func WalletPassword() string {
	return os.Getenv(EnvvarWalletPassword)
}

// apiPasswordFilePath returns the path to the API's password file. The password
// file is stored in the Sia data directory.
func apiPasswordFilePath() string {
	return filepath.Join(SiaDir(), "apipassword")
}

// createAPIPasswordFile creates an api password file in the Sia data directory
// and returns the newly created password
func createAPIPasswordFile() (string, error) {
	err := os.Mkdir(SiaDir(), 0700)
	if err != nil {
		return "", err
	}
	pw := hex.EncodeToString(fastrand.Bytes(16))
	err = ioutil.WriteFile(apiPasswordFilePath(), []byte(pw+"\n"), 0600)
	if err != nil {
		return "", err
	}
	return pw, nil
}

// DefaultMetadataDir returns the default data directory of spd. The values for
// supported operating systems are:
//
// Linux:   $HOME/.scprime
// MacOS:   $HOME/Library/Application Support/ScPrime
// Windows: %LOCALAPPDATA%\ScPrime
func DefaultMetadataDir() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "ScPrime")
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "ScPrime")
	default:
		return filepath.Join(os.Getenv("HOME"), ".scprime")
	}
}

// DefaultSkynetDir returns default data directory for miscellaneous Pubaccess data,
// e.g. pubaccesskeys. The values for supported operating systems are:
//
// Linux:   $HOME/.pubaccess
// MacOS:   $HOME/Library/Application Support/Pubaccess
// Windows: %LOCALAPPDATA%\Pubaccess
func defaultSkynetDir() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "Pubaccess")
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "Pubaccess")
	default:
		return filepath.Join(os.Getenv("HOME"), ".pubaccess")
	}
}

// DefaultSiaPrimeDir returns the default data directory of older ScPrime nodes.
// This method is used to migrate the metadata to the new default location.
// The values for supported operating systems are:
//
// Linux:   $HOME/.siaprime
// MacOS:   $HOME/Library/Application Support/SiaPrime
// Windows: %LOCALAPPDATA%\SiaPrime
func DefaultSiaPrimeDir() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "SiaPrime")
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "SiaPrime")
	default:
		return filepath.Join(os.Getenv("HOME"), ".siaprime")
	}
}
