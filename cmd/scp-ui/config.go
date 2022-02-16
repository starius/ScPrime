package main

import (
	"os"
	"path/filepath"
	"runtime"
	"time"

	"gitlab.com/scpcorp/ScPrime/node"
)

// createNodeParams parses the provided config and creates the corresponding
// node params for the server.
func configNodeParams() node.NodeParams {
	params := node.NodeParams{}
	// Set the modules.
	params.CreateGateway = true
	params.CreateConsensusSet = true
	params.CreateTransactionPool = true
	params.CreateWallet = true
	params.CreateDownloader = true
	params.CreateGui = true
	// Parse remaining fields.
	params.Bootstrap = true
	params.SiaMuxTCPAddress = ":4293"
	params.SiaMuxWSAddress = ":4294"
	params.Dir = defaultScPrimeUiDir()
	params.APIaddr = "localhost:4290"
	params.CheckTokenExpirationFrequency = 1 * time.Hour // default
	params.Headless = false
	return params
}

// defaultScPrimeUiDir returns the default data directory of scp-ui. The values for
// supported operating systems are:
//
// Linux:   $HOME/.scprime-ui
// MacOS:   $HOME/Library/Application Support/ScPrime-UI
// Windows: %LOCALAPPDATA%\ScPrime-UI
func defaultScPrimeUiDir() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "ScPrime-UI")
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "ScPrime-UI")
	default:
		return filepath.Join(os.Getenv("HOME"), ".scprime-ui")
	}
}
