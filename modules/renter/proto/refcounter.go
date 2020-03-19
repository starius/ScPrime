package proto

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"sync"

	"gitlab.com/NebulousLabs/Sia/modules"

	"gitlab.com/NebulousLabs/writeaheadlog"

	"gitlab.com/NebulousLabs/errors"
)

var (
	// ErrInvalidHeaderData is returned when we try to deserialize the header from
	// a []byte with incorrect data
	ErrInvalidHeaderData = errors.New("invalid header data")

	// ErrInvalidSectorNumber is returned when the requested sector doesnt' exist
	ErrInvalidSectorNumber = errors.New("invalid sector given - it does not exist")

	// ErrInvalidVersion is returned when the version of the file we are trying to
	// read does not match the current RefCounterHeaderSize
	ErrInvalidVersion = errors.New("invalid file version")

	// RefCounterVersion defines the latest version of the RefCounter
	RefCounterVersion = [8]byte{1}

	// UpdateNameDelete is the name of an idempotent update that deletes a file
	// from the disk.
	UpdateNameDelete = writeaheadlog.NameDeleteUpdate

	// UpdateNameTruncate is the name of an idempotent update that truncates a
	// refcounter file by a number of sectors.
	UpdateNameTruncate = "RC_TRUNCATE"

	// UpdateNameWriteAt is the name of an idempotent update that writes a
	// value to a position in the file.
	UpdateNameWriteAt = "RC_WRITE_AT"
)

const (
	// RefCounterHeaderSize is the size of the header in bytes
	RefCounterHeaderSize = 8
)

type (
	// RefCounter keeps track of how many references to each sector exist.
	//
	// Once the number of references drops to zero we consider the sector as
	// garbage. We move the sector to end of the data and set the
	// GarbageCollectionOffset to point to it. We can either reuse it to store
	// new data or drop it from the contract at the end of the current period
	// and before the contract renewal.
	RefCounter struct {
		RefCounterHeader

		filepath   string // where the refcounter is persisted on disk
		numSectors uint64 // used for sanity checks before we attempt mutation operations
		wal        *writeaheadlog.WAL
		mu         sync.Mutex

		// While updating the reference counters on this we will also keep the
		// new values in memory, so we can work with them even before they are
		// stored on disk.
		newSectorCounts map[uint64]uint16 // holds the new value of a given counter

		// muUpdates controls who can create and apply updates
		muUpdates sync.Mutex
	}

	// RefCounterHeader contains metadata about the reference counter file
	RefCounterHeader struct {
		Version [8]byte
	}

	// u16 is a utility type for ser/des of uint16 values
	u16 [2]byte
)

// LoadRefCounter loads a refcounter from disk
func LoadRefCounter(path string, wal *writeaheadlog.WAL) (RefCounter, error) {
	// Open the file and start loading the data.
	f, err := os.Open(path)
	if err != nil {
		return RefCounter{}, err
	}
	defer f.Close()

	var header RefCounterHeader
	headerBytes := make([]byte, RefCounterHeaderSize)
	if _, err = f.ReadAt(headerBytes, 0); err != nil {
		return RefCounter{}, errors.AddContext(err, "unable to read from file")
	}
	if err = deserializeHeader(headerBytes, &header); err != nil {
		return RefCounter{}, errors.AddContext(err, "unable to load refcounter header")
	}
	if header.Version != RefCounterVersion {
		return RefCounter{}, errors.AddContext(ErrInvalidVersion, fmt.Sprintf("expected version %d, got version %d", RefCounterVersion, header.Version))
	}
	fi, err := os.Stat(path)
	if err != nil {
		return RefCounter{}, errors.AddContext(err, "failed to read file stats")
	}
	numSectors := uint64((fi.Size() - RefCounterHeaderSize) / 2)
	return RefCounter{
		RefCounterHeader: header,
		filepath:         path,
		numSectors:       numSectors,
		wal:              wal,
		newSectorCounts:  make(map[uint64]uint16),
	}, nil
}

// NewRefCounter creates a new sector reference counter file to accompany
// a contract file
func NewRefCounter(path string, numSec uint64, wal *writeaheadlog.WAL) (RefCounter, error) {
	h := RefCounterHeader{
		Version: RefCounterVersion,
	}
	updateHeader := writeaheadlog.WriteAtUpdate(path, 0, serializeHeader(h))

	b := make([]byte, numSec*2)
	for i := uint64(0); i < numSec; i++ {
		binary.LittleEndian.PutUint16(b[i*2:i*2+2], 1)
	}
	updateCounters := writeaheadlog.WriteAtUpdate(path, RefCounterHeaderSize, b)

	err := wal.CreateAndApplyTransaction(writeaheadlog.ApplyUpdates, updateHeader, updateCounters)
	return RefCounter{
		RefCounterHeader: h,
		filepath:         path,
		numSectors:       numSec,
		wal:              wal,
		newSectorCounts:  make(map[uint64]uint16),
	}, err
}

// Append appends one counter to the end of the refcounter file and
// initializes it with `1`
func (rc *RefCounter) Append() writeaheadlog.Update {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.numSectors++
	rc.newSectorCounts[rc.numSectors-1] = 1
	return createWriteAtUpdate(rc.filepath, rc.numSectors-1, 1)
}

// Count returns the number of references to the given sector
func (rc *RefCounter) Count(secIdx uint64) (uint16, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.readCount(secIdx)
}

// CreateAndApplyTransaction is a helper method that creates a writeaheadlog
// transaction and applies it.
func (rc *RefCounter) CreateAndApplyTransaction(updates ...writeaheadlog.Update) error {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	// Create the writeaheadlog transaction.
	txn, err := rc.wal.NewTransaction(updates)
	if err != nil {
		return errors.AddContext(err, "failed to create wal txn")
	}
	// No extra setup is required. Signal that it is done.
	if err := <-txn.SignalSetupComplete(); err != nil {
		return errors.AddContext(err, "failed to signal setup completion")
	}
	// Apply the updates.
	if err := applyUpdates(rc.filepath, updates...); err != nil {
		return errors.AddContext(err, "failed to apply updates")
	}
	// Updates are applied. Let the writeaheadlog know.
	if err := txn.SignalUpdatesApplied(); err != nil {
		return errors.AddContext(err, "failed to signal that updates are applied")
	}
	return nil
}

// Decrement decrements the reference counter of a given sector. The sector
// is specified by its sequential number (secIdx).
// Returns the updated number of references or an error.
func (rc *RefCounter) Decrement(secIdx uint64) (writeaheadlog.Update, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if secIdx > rc.numSectors-1 {
		return writeaheadlog.Update{}, ErrInvalidSectorNumber
	}
	count, err := rc.readCount(secIdx)
	if err != nil {
		return writeaheadlog.Update{}, errors.AddContext(err, "failed to read count")
	}
	if count == 0 {
		return writeaheadlog.Update{}, errors.New("sector count underflow")
	}
	count--
	rc.newSectorCounts[secIdx] = count
	return createWriteAtUpdate(rc.filepath, secIdx, count), nil
}

// DeleteRefCounter deletes the counter's file from disk
func (rc *RefCounter) DeleteRefCounter() writeaheadlog.Update {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return createDeleteUpdate(rc.filepath)
}

// DropSectors removes the last numSec sector counts from the refcounter file
func (rc *RefCounter) DropSectors(numSec uint64) (writeaheadlog.Update, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if numSec > rc.numSectors {
		return writeaheadlog.Update{}, ErrInvalidSectorNumber
	}
	rc.numSectors -= numSec
	return createTruncateUpdate(rc.filepath, rc.numSectors), nil
}

// Increment increments the reference counter of a given sector. The sector
// is specified by its sequential number (secIdx).
// Returns the updated number of references or an error.
func (rc *RefCounter) Increment(secIdx uint64) (writeaheadlog.Update, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if secIdx > rc.numSectors-1 {
		return writeaheadlog.Update{}, ErrInvalidSectorNumber
	}
	count, err := rc.readCount(secIdx)
	if err != nil {
		return writeaheadlog.Update{}, errors.AddContext(err, "failed to read count")
	}
	if count == math.MaxUint16 {
		return writeaheadlog.Update{}, errors.New("sector count overflow")
	}
	count++
	rc.newSectorCounts[secIdx] = count
	return createWriteAtUpdate(rc.filepath, secIdx, count), nil
}

// StartUpdate acquires a lock, ensuring the caller is the only one currently
// allowed to perform updates on this refcounter file.
func (rc *RefCounter) StartUpdate() {
	rc.muUpdates.Lock()
}

// Swap swaps the two sectors at the given indices
func (rc *RefCounter) Swap(firstIdx, secondIdx uint64) ([]writeaheadlog.Update, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if firstIdx > rc.numSectors-1 || secondIdx > rc.numSectors-1 {
		return []writeaheadlog.Update{}, ErrInvalidSectorNumber
	}
	firstVal, err := rc.readCount(firstIdx)
	if err != nil {
		return []writeaheadlog.Update{}, errors.AddContext(err, "failed to read count")
	}
	secondVal, err := rc.readCount(secondIdx)
	if err != nil {
		return []writeaheadlog.Update{}, errors.AddContext(err, "failed to read count")
	}
	rc.newSectorCounts[firstIdx] = secondVal
	rc.newSectorCounts[secondIdx] = firstVal
	return []writeaheadlog.Update{
		createWriteAtUpdate(rc.filepath, firstIdx, secondVal),
		createWriteAtUpdate(rc.filepath, secondIdx, firstVal),
	}, nil
}

// UpdateApplied cleans up temporary data and releases the update lock, thus
// allowing other actors to acquire it in order to update the refcounter.
func (rc *RefCounter) UpdateApplied() {
	rc.mu.Lock()
	// clean up the temp counts
	rc.newSectorCounts = make(map[uint64]uint16)
	rc.mu.Unlock()
	// release the update lock
	rc.muUpdates.Unlock()
}

// readCount reads the given sector count either from disk (if there are no
// pending updates) or from the in-memory cache (if there are).
func (rc *RefCounter) readCount(secIdx uint64) (uint16, error) {
	// check if the secIdx is a valid sector index based on the number of
	// sectors in the file
	if secIdx > rc.numSectors-1 {
		return 0, ErrInvalidSectorNumber
	}
	// check if the value is being changed by a pending update
	if count, ok := rc.newSectorCounts[secIdx]; ok {
		return count, nil
	}
	// read the value from disk
	f, err := os.Open(rc.filepath)
	if err != nil {
		return 0, errors.AddContext(err, "failed to open the refcounter file")
	}
	defer f.Close()

	var b u16
	if _, err = f.ReadAt(b[:], int64(offset(secIdx))); err != nil {
		return 0, errors.AddContext(err, "failed to read from the refcounter file")
	}
	return binary.LittleEndian.Uint16(b[:]), nil
}

// applyDeleteUpdate parses and applies a Delete update.
func applyDeleteUpdate(update writeaheadlog.Update) error {
	return writeaheadlog.ApplyDeleteUpdate(update)
}

// applyTruncateUpdate parses and applies a Truncate update.
func applyTruncateUpdate(f *os.File, u writeaheadlog.Update) error {
	if u.Name != UpdateNameTruncate {
		return fmt.Errorf("applyAppendTruncate called on update of type %v", u.Name)
	}
	// Decode update.
	_, newNumSec, err := readTruncateUpdate(u)
	if err != nil {
		return err
	}
	// Truncate the file to the needed size.
	if err = f.Truncate(RefCounterHeaderSize + int64(newNumSec)*2); err != nil {
		return err
	}
	return f.Sync()
}

// applyUpdates takes a list of WAL updates and applies them.
func applyUpdates(path string, updates ...writeaheadlog.Update) error {
	// If there is a Delete update in the list then all updates prior to that
	// are moot. That's why we handle deletes separately.
	lastDelPos := -1
	for i, u := range updates {
		if u.Name == UpdateNameDelete {
			lastDelPos = i
		}
	}
	if lastDelPos > -1 {
		if err := applyDeleteUpdate(updates[lastDelPos]); err != nil {
			return err
		}
		updates = updates[lastDelPos+1:]
	}
	// if the last update was a Delete then just return - we're done
	if len(updates) == 0 {
		return nil
	}
	// Now that the deletes are done, open the file
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, modules.DefaultFilePerm)
	if err != nil {
		return errors.AddContext(err, "failed to open refcounter file in order to apply updates")
	}
	defer f.Close()
	// Execute all non-Delete updates
	for _, update := range updates {
		var err error
		switch update.Name {
		case UpdateNameTruncate:
			err = applyTruncateUpdate(f, update)
		case UpdateNameWriteAt:
			err = applyWriteAtUpdate(f, update)
		default:
			err = fmt.Errorf("unknown update type: %v", update.Name)
		}
		if err != nil {
			return err
		}
	}
	return f.Sync()
}

// applyWriteAtUpdate parses and applies a WriteAt update.
func applyWriteAtUpdate(f *os.File, u writeaheadlog.Update) error {
	if u.Name != UpdateNameWriteAt {
		return fmt.Errorf("applyAppendWriteAt called on update of type %v", u.Name)
	}
	// Decode update.
	_, secIdx, value, err := readWriteAtUpdate(u)

	// Write the value to disk.
	var b u16
	binary.LittleEndian.PutUint16(b[:], value)
	if _, err = f.WriteAt(b[:], int64(offset(secIdx))); err != nil {
		return err
	}
	return f.Sync()
}

// createDeleteUpdate is a helper function which creates a writeaheadlog update
// for deleting a given refcounter file.
func createDeleteUpdate(path string) writeaheadlog.Update {
	return writeaheadlog.DeleteUpdate(path)
}

// createTruncateUpdate is a helper function which creates a writeaheadlog
// update for truncating a number of sectors from the end of the file.
func createTruncateUpdate(path string, newNumSec uint64) writeaheadlog.Update {
	b := make([]byte, 8+4+len(path))
	binary.LittleEndian.PutUint64(b[:8], newNumSec)
	binary.LittleEndian.PutUint32(b[8:12], uint32(len(path)))
	copy(b[12:12+len(path)], path)
	return writeaheadlog.Update{
		Name:         UpdateNameTruncate,
		Instructions: b,
	}
}

// createWriteAtUpdate is a helper function which creates a writeaheadlog
// update for swapping the values of two positions in the file.
func createWriteAtUpdate(path string, secIdx uint64, value uint16) writeaheadlog.Update {
	b := make([]byte, 8+2+4+len(path))
	binary.LittleEndian.PutUint64(b[:8], secIdx)
	binary.LittleEndian.PutUint16(b[8:10], value)
	binary.LittleEndian.PutUint32(b[10:14], uint32(len(path)))
	copy(b[14:14+len(path)], path)
	return writeaheadlog.Update{
		Name:         UpdateNameWriteAt,
		Instructions: b,
	}
}

// deserializeHeader deserializes a header from []byte
func deserializeHeader(b []byte, h *RefCounterHeader) error {
	if uint64(len(b)) < RefCounterHeaderSize {
		return ErrInvalidHeaderData
	}
	copy(h.Version[:], b[:8])
	return nil
}

// offset calculates the byte offset of the sector counter in the file on disk
func offset(secIdx uint64) uint64 {
	return RefCounterHeaderSize + secIdx*2
}

// readTruncateUpdate decodes a Truncate update
func readTruncateUpdate(u writeaheadlog.Update) (path string, newNumSec uint64, err error) {
	if len(u.Instructions) < 20 {
		err = errors.New("instructions slice of update is too short to contain the size and path")
		return
	}
	newNumSec = binary.LittleEndian.Uint64(u.Instructions[:8])
	pathLen := int32(binary.LittleEndian.Uint32(u.Instructions[8:12]))
	path = string(u.Instructions[12 : 12+pathLen])
	return
}

// readWriteAtUpdate decodes a WriteAt update
func readWriteAtUpdate(u writeaheadlog.Update) (path string, secIdx uint64, value uint16, err error) {
	if len(u.Instructions) < 20 {
		err = errors.New("instructions slice of update is too short to contain the size and path")
		return
	}
	secIdx = binary.LittleEndian.Uint64(u.Instructions[:8])
	value = binary.LittleEndian.Uint16(u.Instructions[8:10])
	pathLen := int64(binary.LittleEndian.Uint32(u.Instructions[10:14]))
	path = string(u.Instructions[14 : 14+pathLen])
	return
}

// serializeHeader serializes a header to []byte
func serializeHeader(h RefCounterHeader) []byte {
	b := make([]byte, RefCounterHeaderSize)
	copy(b[:8], h.Version[:])
	return b
}
