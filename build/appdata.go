package build

import (
	"os"
	"path/filepath"
	"runtime"
)

// DefaultSiaDir returns the default data directory of siad. The values for
// supported operating systems are:
//
// Linux:   $HOME/.sia
// MacOS:   $HOME/Library/Application Support/Sia
// Windows: %LOCALAPPDATA%\Sia
func DefaultSiaDir() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "SiaPrime")
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "SiaPrime")
	default:
		return filepath.Join(os.Getenv("HOME"), ".siaprime")
	}
}

// APIPasswordFile returns the path to the API's password file given a Sia
// directory.
func APIPasswordFile(siaDir string) string {
	return filepath.Join(siaDir, "apipassword")
}
