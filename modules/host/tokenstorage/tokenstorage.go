package tokenstorage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/host/tokenstorage/tokenstate"
	"gitlab.com/scpcorp/ScPrime/types"
	"gitlab.com/zer0main/eventsourcing"
	"gitlab.com/zer0main/filestorage"
)

var (
	// ErrInsufficientResource is an error indicating lack of resources on the token.
	ErrInsufficientResource = errors.New("insufficient token resource for this operation")
)

// TODO: the current version of tokens storage does not support reverting blocks.
// If contracts related to `TopUpToken` RPC are reverted, all the tokens resources remain in the storage.

const (
	persistDelay = 1 * time.Second
	logFileName  = "token_storage.log"
)

// TokenStorageInfo represent data about storage resource.
type TokenStorageInfo struct {
	Storage        int64 // sectors * second
	LastChangeTime time.Time
	SectorsNum     uint64
}

// TokenRecord include information about token record.
type TokenRecord struct {
	DownloadBytes    int64
	UploadBytes      int64
	SectorAccesses   int64
	TokenStorageInfo TokenStorageInfo
}

type storage interface {
	Lock(ctx context.Context) error
	Unlock(ctx context.Context) error
	AppendMeta(ctx context.Context, callback func(w io.Writer) error) error
}

// TokenStorage - storage of tokens for prepaid downloads.
type TokenStorage struct {
	storage storage
	state   *tokenstate.State
	stateMu sync.Mutex // For in-memory only.
	metaMu  sync.Mutex // For drainEventsQueue (involving IO).

	logFile *os.File

	eventsQueue []interface{}

	storageManager modules.StorageManager

	closed bool
}

// NewTokenStorage - create new storage of tokens for prepaid downloads.
func NewTokenStorage(stManager modules.StorageManager, dir string) (*TokenStorage, error) {
	storage := filestorage.NewFileStorage(dir)
	ctx := context.Background()

	if err := storage.Lock(ctx); err != nil {
		return nil, fmt.Errorf("failed to lock: %w", err)
	}

	var state *tokenstate.State
	state, err := tokenstate.NewState(dir)
	if err != nil {
		return nil, err
	}
	err = storage.LoadMeta(ctx, func(r io.Reader) error {
		return state.LoadHistory(r)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load history: %w", err)
	}
	logPath := filepath.Join(dir, logFileName)
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s for writing: %w", logPath, err)
	}
	log.SetOutput(logFile)
	log.Println("Created token storage")
	s := &TokenStorage{
		storage:        storage,
		state:          state,
		storageManager: stManager,
		logFile:        logFile,
	}
	return s, nil
}

// Close - drain event queue and close storage.
func (t *TokenStorage) Close(ctx context.Context) error {
	t.stateMu.Lock()
	t.closed = true
	t.stateMu.Unlock()
	err := t.state.Close()
	if err != nil {
		return err
	}
	if err = t.drainEventsQueue(); err != nil {
		return fmt.Errorf("drainEventsQueue: %w", err)
	}
	if err = t.storage.Unlock(ctx); err != nil {
		return fmt.Errorf("failed to unlock: %w", err)
	}
	if err = t.logFile.Close(); err != nil {
		return fmt.Errorf("failed to close log file: %w", err)
	}
	return nil
}

// drainEventsQueue writes all pending events to hard drive (append to file metadata.json).
// Mutex stateMu must NOT be locked when this function is called.
func (t *TokenStorage) drainEventsQueue() error {
	return eventsourcing.DrainEventsQueue(context.Background(), &t.stateMu, &t.metaMu, &t.eventsQueue, t.storage)
}

// TokenRecord return token record by id.
func (t *TokenStorage) TokenRecord(id types.TokenID) (TokenRecord, error) {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.closed {
		return TokenRecord{}, fmt.Errorf("token storage closed")
	}
	record, exist := t.state.Tokens[id]
	if !exist {
		return TokenRecord{}, nil
	}
	return TokenRecord{
		DownloadBytes:  record.DownloadBytes,
		UploadBytes:    record.UploadBytes,
		SectorAccesses: record.SectorAccesses,
		TokenStorageInfo: TokenStorageInfo{
			Storage:        record.TokenInfo.Storage,
			LastChangeTime: record.TokenInfo.LastChangeTime,
			SectorsNum:     record.TokenInfo.SectorsNum,
		},
	}, nil
}

// RecordDownload set token record fields.
func (t *TokenStorage) RecordDownload(id types.TokenID, downloadBytes, sectorAccesses int64) error {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.closed {
		return fmt.Errorf("token storage closed")
	}
	t.applyEvent(&tokenstate.Event{EventTokenDownload: &tokenstate.EventTokenDownload{
		TokenID:        id,
		DownloadBytes:  downloadBytes,
		SectorAccesses: sectorAccesses,
	}, Time: time.Now()})
	return nil
}

// AddResources - add resource to token.
func (t *TokenStorage) AddResources(id types.TokenID, resourceType types.Specifier, amount int64) error {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.closed {
		return fmt.Errorf("token storage closed")
	}
	t.applyEvent(&tokenstate.Event{EventTopUp: &tokenstate.EventTopUp{
		TokenID:        id,
		ResourceType:   resourceType,
		ResourceAmount: amount,
	}, Time: time.Now()})
	return nil
}

// AddSectors add sectors to token.
func (t *TokenStorage) AddSectors(id types.TokenID, sectorsIDs []crypto.Hash, time time.Time) error {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.closed {
		return fmt.Errorf("token storage closed")
	}
	log.Printf("Adding %d sectors to token %s", len(sectorsIDs), id.String())
	t.applyEvent(&tokenstate.Event{EventAddSectors: &tokenstate.EventAddSectors{
		TokenID:    id,
		SectorsIDs: crypto.ConvertHashesToByteSlices(sectorsIDs),
	}, Time: time})
	return nil
}

// ListSectorIDs returns list of sector ids.
func (t *TokenStorage) ListSectorIDs(id types.TokenID, pageID string, limit int) (sectorIDs []crypto.Hash, nextPageID string, err error) {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.closed {
		return nil, "", fmt.Errorf("token storage closed")
	}
	return t.state.GetLimitedSectors(id, pageID, limit)
}

// RemoveSpecificSectors removes only passed sectors ids.
func (t *TokenStorage) RemoveSpecificSectors(id types.TokenID, sectorIDs []crypto.Hash, time time.Time) error {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.closed {
		return fmt.Errorf("token storage closed")
	}
	log.Printf("Received request to remove sectors of token %s", id.String())
	// Exclude nonexistent sectors before creating event to prevent attacks based on filling events history with garbage.
	existingSectors, _, err := t.state.HasSectors(id, sectorIDs)
	if err != nil {
		return fmt.Errorf("check has sector: %w", err)
	}
	if len(existingSectors) == 0 {
		return nil
	}
	t.applyEvent(&tokenstate.Event{EventRemoveSpecificSectors: &tokenstate.EventRemoveSpecificSectors{
		TokenID:    id,
		SectorsIDs: crypto.ConvertHashesToByteSlices(existingSectors),
	}, Time: time})

	// Remove sectors from disk.
	go func() {
		if err := t.storageManager.RemoveSectorBatch(existingSectors); err != nil {
			log.Printf("Failed to remove sectors: %v", err)
		}
	}()
	return nil
}

// AttachSectors attach sector to contract.
func (t *TokenStorage) AttachSectors(sectorIDs map[types.TokenID][]crypto.Hash, callTime time.Time) error {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.closed {
		return fmt.Errorf("token storage closed")
	}
	var attachSectors []tokenstate.AttachSectorsData
	for token, ids := range sectorIDs {
		_, hasSectors, err := t.state.HasSectors(token, ids)
		if err != nil {
			return fmt.Errorf("HasSectors failed: %w", err)
		}
		if !hasSectors {
			return fmt.Errorf("some sectors of token %s don't exist", token.String())
		}
		if enough := t.state.EnoughStorageResource(token, int64(-len(ids)), time.Now()); !enough {
			return ErrInsufficientResource
		}
		for _, id := range ids {
			attachSectors = append(attachSectors, tokenstate.AttachSectorsData{
				TokenID:  token,
				SectorID: id.Bytes(),
			})
		}
	}
	t.applyEvent(&tokenstate.Event{EventAttachSectors: &tokenstate.EventAttachSectors{
		TokensSectors: attachSectors,
	}, Time: callTime})
	return nil
}

// EnoughStorageResource checks if there is enough storage resource on token to store existing
// sectors and new ones, return false if not enough.
func (t *TokenStorage) EnoughStorageResource(id types.TokenID, sectorsNum int64, now time.Time) (bool, error) {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.closed {
		return false, fmt.Errorf("token storage closed")
	}
	token := t.state.Tokens[id]
	log.Printf("EnoughStorageResource: token %s; token.TokenInfo.Storage: %d; time diff: %s; token.TokenInfo.SectorsNum: %d", id.String(), token.TokenInfo.Storage, now.Sub(token.TokenInfo.LastChangeTime).String(), token.TokenInfo.SectorsNum)
	return t.state.EnoughStorageResource(id, sectorsNum, now), nil
}

// applyEvent applies an event to State and adds it to a queue to be added to metadata.json.
// Mutex stateMu must be locked when this function is called.
func (t *TokenStorage) applyEvent(event *tokenstate.Event) {
	if t.closed {
		panic("an attempt to add an event while the repo is closed")
	}
	t.state.Apply(event)
	t.eventsQueue = append(t.eventsQueue, event)
	if len(t.eventsQueue) == 1 {
		// No waiting events yet, create a delayed drainEventsQueue task.
		time.AfterFunc(persistDelay, func() {
			if err := t.drainEventsQueue(); err != nil {
				log.Printf("drainEventsQueue failed: %v", err)
			}
		})
	}
}

// CheckExpiration remove sectors from token when token storage resource ends.
func (t *TokenStorage) CheckExpiration(frequency time.Duration, done chan bool) {
	ticker := time.NewTicker(frequency)
	log.Printf("CheckExpiration started with frequency %s", frequency.String())
	for {
		select {
		case <-done:
			ticker.Stop()
			return

		case <-ticker.C:
			t.checkExpiration()
		}
	}
}

func (t *TokenStorage) checkExpiration() {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.closed {
		return
	}

	log.Println("checkExpiration is called")
	for token := range t.state.Tokens {
		if enough := t.state.EnoughStorageResource(token, 0, time.Now()); enough {
			log.Printf("Token %s has enough storage, don't remove", token.String())
			continue
		}
		sectors, err := t.state.GetSectors(token)
		if err != nil {
			log.Printf("Failed to get sectors of token %s", token.String())
			continue
		}

		log.Printf("Token %s does not have enough storage resource, removing all its sectors...", token.String())

		t.applyEvent(&tokenstate.Event{EventRemoveAllSectors: &tokenstate.EventRemoveAllSectors{
			TokenID:    token,
			SectorsIDs: crypto.ConvertHashesToByteSlices(sectors),
		}, Time: time.Now()})

		// Removing a lot of sectors might take time, we can't do it under stateMu,
		// so we do it in a separate goroutine here. IDs of these sectors are
		// already removed from state, so there is no problem with returning
		// from this function before fully removing sectors data from disk.
		go func(token types.TokenID) {
			if err := t.storageManager.RemoveSectorBatch(sectors); err != nil {
				log.Printf("Failed to remove sectors of depleted token %s: %v", token.String(), err)
			}
		}(token)
	}
}
