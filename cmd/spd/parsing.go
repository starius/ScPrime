package main

import (
	"strings"

	"gitlab.com/SiaPrime/SiaPrime/node"
)

// createNodeParams parses the provided config and creates the corresponding
// node params for the server.
func parseModules(config Config) node.NodeParams {
	params := node.NodeParams{}
	// Parse the modules.
	if strings.Contains(config.Spd.Modules, "g") {
		params.CreateGateway = true
	}
	if strings.Contains(config.Spd.Modules, "c") {
		params.CreateConsensusSet = true
	}
	if strings.Contains(config.Spd.Modules, "e") {
		params.CreateExplorer = true
	}
	if strings.Contains(config.Spd.Modules, "t") {
		params.CreateTransactionPool = true
	}
	if strings.Contains(config.Spd.Modules, "w") {
		params.CreateWallet = true
	}
	if strings.Contains(config.Spd.Modules, "m") {
		params.CreateMiner = true
	}
	if strings.Contains(config.Spd.Modules, "h") {
		params.CreateHost = true
	}
	if strings.Contains(config.Spd.Modules, "r") {
		params.CreateRenter = true
	}
	// Parse remaining fields.
	params.Bootstrap = !config.Spd.NoBootstrap
	params.HostAddress = config.Spd.HostAddr
	params.RPCAddress = config.Spd.RPCaddr
	params.Dir = config.Spd.SiaDir
	return params
}
