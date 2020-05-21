package pubaccessblacklist

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/encoding"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/persist"
	"gitlab.com/scpcorp/ScPrime/types"

	"gitlab.com/NebulousLabs/errors"
)

const (
	// persistFile is the name of the persist file
	persistFile string = "pubaccessblacklist"

	// persistSize is the size of a persisted merkleroot in the blacklist. It is
	// the length of `merkleroot` plus the `listed` flag (32 + 1).
	persistSize uint64 = 33
)

var (
	// metadataHeader is the header of the metadata for the persist file
	metadataHeader = types.NewSpecifier("PublicBlacklist\n")

	// metadataVersion is the version of the persistence file
	metadataVersion = types.NewSpecifier("v1.4.3\n")
)

type (
	// SkynetBlacklist manages a set of blacklisted publinks by tracking the
	// merkleroots and persists the list to disk.
	SkynetBlacklist struct {
		staticAop *persist.AppendOnlyPersist

		// merkleRoots is a set of blacklisted links.
		merkleRoots map[crypto.Hash]struct{}

		mu sync.Mutex
	}

	// persistEntry contains a public access blacklist link and whether it should be
	// listed as being in the persistence file.
	persistEntry struct {
		MerkleRoot crypto.Hash
		Listed     bool
	}
)

// New returns an initialized SkynetBlacklist.
func New(persistDir string) (*SkynetBlacklist, error) {
	// Initialize the persistence of the blacklist.
	aop, reader, err := persist.NewAppendOnlyPersist(persistDir, persistFile, metadataHeader, metadataVersion)
	if err != nil {
		return nil, errors.AddContext(err, fmt.Sprintf("unable to initialize the public access blacklist persistence at '%v'", aop.FilePath()))
	}

	sb := &SkynetBlacklist{
		staticAop: aop,
	}
	blacklist, err := unmarshalObjects(reader)
	if err != nil {
		return nil, errors.AddContext(err, "unable to unmarshal persist objects")
	}
	sb.merkleRoots = blacklist

	return sb, nil
}

// Blacklist returns the merkleroots that are blacklisted
func (sb *SkynetBlacklist) Blacklist() []crypto.Hash {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	var blacklist []crypto.Hash
	for mr := range sb.merkleRoots {
		blacklist = append(blacklist, mr)
	}
	return blacklist
}

// Close closes and frees associated resources.
func (sb *SkynetBlacklist) Close() error {
	return sb.staticAop.Close()
}

// IsBlacklisted indicates if a publink is currently blacklisted
func (sb *SkynetBlacklist) IsBlacklisted(publink modules.Publink) bool {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	_, ok := sb.merkleRoots[publink.MerkleRoot()]
	return ok
}

// UpdateBlacklist updates the list of publinks that are blacklisted.
func (sb *SkynetBlacklist) UpdateBlacklist(additions, removals []modules.Publink) error {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	buf, err := sb.marshalObjects(additions, removals)
	if err != nil {
		return errors.AddContext(err, fmt.Sprintf("unable to update pubaccess blacklist persistence at '%v'", sb.staticAop.FilePath()))
	}
	_, err = sb.staticAop.Write(buf.Bytes())
	return errors.AddContext(err, fmt.Sprintf("unable to update pubaccess blacklist persistence at '%v'", sb.staticAop.FilePath()))
}

// marshalObjects marshals the given objects into a byte buffer.
//
// NOTE: this method does not check for duplicate additions or removals
func (sb *SkynetBlacklist) marshalObjects(additions, removals []modules.Publink) (bytes.Buffer, error) {
	// Create buffer for encoder
	var buf bytes.Buffer
	// Create and encode the persist links
	listed := true
	for _, publink := range additions {
		// Add publink merkleroot to map
		mr := publink.MerkleRoot()
		sb.merkleRoots[mr] = struct{}{}

		// Marshal the update
		pe := persistEntry{mr, listed}
		bytes := encoding.Marshal(pe)
		buf.Write(bytes)
	}
	listed = false
	for _, publink := range removals {
		// Remove publink merkleroot from map
		mr := publink.MerkleRoot()
		delete(sb.merkleRoots, mr)

		// Marshal the update
		pe := persistEntry{mr, listed}
		bytes := encoding.Marshal(pe)
		buf.Write(bytes)
	}

	return buf, nil
}

// unmarshalObjects unmarshals the sia encoded objects.
func unmarshalObjects(reader io.Reader) (map[crypto.Hash]struct{}, error) {
	blacklist := make(map[crypto.Hash]struct{})
	// Unmarshal blacklisted links one by one until EOF.
	var offset uint64
	for {
		buf := make([]byte, persistSize)
		_, err := io.ReadFull(reader, buf)
		if errors.Contains(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		var pe persistEntry
		err = encoding.Unmarshal(buf, &pe)
		if err != nil {
			return nil, err
		}
		offset += persistSize

		if !pe.Listed {
			delete(blacklist, pe.MerkleRoot)
			continue
		}
		blacklist[pe.MerkleRoot] = struct{}{}
	}
	return blacklist, nil
}
