package siatest

import (
	"gitlab.com/scpcorp/ScPrime/modules"
)

type (
	// RemoteDir is a helper struct that represents a directory on the ScPrime
	// network.
	RemoteDir struct {
		siapath modules.SiaPath
	}
)

// SiaPath returns the siapath of a remote directory.
func (rd *RemoteDir) SiaPath() modules.SiaPath {
	return rd.siapath
}
