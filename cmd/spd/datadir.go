// datadir.go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gitlab.com/scpcorp/ScPrime/build"
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

// migrateDataDir migrates existing metadata directory to new location
func migrateDataDir() error {
	fmt.Printf("Migrating metadata from %v to %v\n", defaultSiaPrimeDir(), build.DefaultMetadataDir())
	return os.Rename(defaultSiaPrimeDir(), build.DefaultMetadataDir())
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
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "ScPrime")
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "ScPrime")
	default:
		return filepath.Join(os.Getenv("HOME"), ".siaprime")
	}
}
