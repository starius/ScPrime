// datadir.go
package build

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Check for Metadata directory presence and try to migrate if it does not
// exist

// dirExists checks if a directory exists.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

// moveDataDir migrates existing metadata directory to new location
func moveDataDir() error {
	fmt.Printf("Migrating metadata from %v to %v\n", defaultSiaPrimeDir(), defaultMetadataDir())
	return os.Rename(defaultSiaPrimeDir(), defaultMetadataDir())
}

// defaultSiaPrimeDir returns the default data directory of older ScPrime nodes.
// This method is used to migrate the metadata to the new default location.
// The values for supported operating systems are:
//
// Linux:   $HOME/.siaprime
// MacOS:   $HOME/Library/Application Support/ScPrime
// Windows: %LOCALAPPDATA%\ScPrime
func defaultSiaPrimeDir() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "SiaPrime")
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "SiaPrime")
	default:
		return filepath.Join(os.Getenv("HOME"), ".siaprime")
	}
}
