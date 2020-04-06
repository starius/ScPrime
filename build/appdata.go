package build

import (
	"os"
	"path/filepath"
	"runtime"
)

// DefaultSiaDir returns the default data directory of siad. The values for
// supported operating systems are:
//
// Linux:   $HOME/.scprime
// MacOS:   $HOME/Library/Application Support/ScPrime
// Windows: %LOCALAPPDATA%\ScPrime
func DefaultSiaDir() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "ScPrime")
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "ScPrime")
	default:
		return filepath.Join(os.Getenv("HOME"), ".scprime")
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

// DefaultSkynetDir returns default data directory for miscellaneous Skynet data,
// e.g. skykeys. The values for supported operating systems are:
//
// Linux:   $HOME/.skynet
// MacOS:   $HOME/Library/Application Support/Skynet
// Windows: %LOCALAPPDATA%\Skynet
func DefaultSkynetDir() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "Skynet")
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "Skynet")
	default:
		return filepath.Join(os.Getenv("HOME"), ".skynet")
	}
}

// APIPasswordFile returns the path to the API's password file given a Sia
// directory.
func APIPasswordFile(siaDir string) string {
	return filepath.Join(siaDir, "apipassword")
}
