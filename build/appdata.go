package build

import (
	"os"
	"path/filepath"
	"runtime"
)

// DefaultMetadataDir returns the default data directory of siad. The values for
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
// e.g. skykeys. The values for supported operating systems are:
//
// Linux:   $HOME/.pubaccess
// MacOS:   $HOME/Library/Application Support/Pubaccess
// Windows: %LOCALAPPDATA%\Pubaccess
func DefaultSkynetDir() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "Pubaccess")
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "Pubaccess")
	default:
		return filepath.Join(os.Getenv("HOME"), ".pubaccess")
	}
}

// APIPasswordFile returns the path to the API's password file given a Sia
// directory.
func APIPasswordFile(siaDir string) string {
	return filepath.Join(siaDir, "apipassword")
}
