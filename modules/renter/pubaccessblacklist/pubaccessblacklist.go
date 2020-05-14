package pubaccessblacklist

import (
	"fmt"
	"sync"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"

	"gitlab.com/NebulousLabs/errors"
)

// SkynetBlacklist manages a set of blacklisted publinks by tracking the
// merkleroots and persists the list to disk
type SkynetBlacklist struct {
	merkleroots      map[crypto.Hash]struct{}
	persistLength    int64
	staticPersistDir string

	mu sync.Mutex
}

// New creates a new SkynetBlacklist
func New(persistDir string) (*SkynetBlacklist, error) {
	sb := &SkynetBlacklist{
		merkleroots:      make(map[crypto.Hash]struct{}),
		staticPersistDir: persistDir,
	}

	// Initialize the persistence of the blacklist
	err := sb.callInitPersist()
	if err != nil {
		return nil, errors.AddContext(err, fmt.Sprintf("unable to initialize the pubaccess blacklist persistence at '%v'", sb.FilePath()))
	}

	return sb, nil
}

// Blacklist returns the merkleroots that are blacklisted
func (sb *SkynetBlacklist) Blacklist() []crypto.Hash {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	var blacklist []crypto.Hash
	for mr := range sb.merkleroots {
		blacklist = append(blacklist, mr)
	}
	return blacklist
}

// IsBlacklisted indicates if a publink is currently blacklisted
func (sb *SkynetBlacklist) IsBlacklisted(publink modules.Publink) bool {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	_, ok := sb.merkleroots[publink.MerkleRoot()]
	return ok
}

// UpdateSkynetBlacklist updates the list of skylinks that are blacklisted
func (sb *SkynetBlacklist) UpdateSkynetBlacklist(additions, removals []modules.Publink) error {
	err := sb.callUpdateAndAppend(additions, removals)
	return errors.AddContext(err, fmt.Sprintf("unable to update pubaccess blacklist persistence at '%v'", sb.FilePath()))
}
