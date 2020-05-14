package pubaccessportals

import (
	"fmt"
	"sync"

	"gitlab.com/NebulousLabs/errors"

	"gitlab.com/scpcorp/ScPrime/modules"
)

var (
	// ErrSkynetPortalsValidation is the error returned when validation of
	// changes to the Pubaccess portals list fails.
	ErrSkynetPortalsValidation = errors.New("could not validate additions and removals")
)

// SkynetPortals manages a list of known Pubaccess portals by persisting the list
// to disk.
type SkynetPortals struct {
	portals          map[modules.NetAddress]bool
	persistLength    int64
	staticPersistDir string

	mu sync.Mutex
}

// New creates a new SkynetPortals.
func New(persistDir string) (*SkynetPortals, error) {
	sp := &SkynetPortals{
		portals:          make(map[modules.NetAddress]bool),
		staticPersistDir: persistDir,
	}

	// Initialize the persistence of the portals list
	err := sp.callInitPersist()
	if err != nil {
		return nil, errors.AddContext(err, fmt.Sprintf("unable to initialize the pubaccess portal list persistence at '%v'", sp.FilePath()))
	}

	return sp, nil
}

// Portals returns the list of known Pubaccess portals.
func (sp *SkynetPortals) Portals() []modules.SkynetPortal {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	var portals []modules.SkynetPortal
	for addr, public := range sp.portals {
		portal := modules.SkynetPortal{
			Address: addr,
			Public:  public,
		}
		portals = append(portals, portal)
	}
	return portals
}

// UpdateSkynetPortals updates the list of known Pubaccess portals.
func (sp *SkynetPortals) UpdateSkynetPortals(additions []modules.SkynetPortal, removals []modules.NetAddress) error {
	err := sp.callUpdateAndAppend(additions, removals)
	return errors.AddContext(err, fmt.Sprintf("unable to update pubaccess portal list persistence at '%v'", sp.FilePath()))
}
