package tokenstorage

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/host/tokenstorage/tokenstate"
	"gitlab.com/scpcorp/ScPrime/types"
	"gitlab.com/zer0main/eventsourcing"
	"gitlab.com/zer0main/filestorage"
)

// TODO: the current version of tokens storage does not support reverting blocks.
// If contracts related to `TopUpToken` RPC are reverted, all the tokens resources remain in the storage.

const persistDelay = 1 * time.Second

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

// AttachSectorsData include information about token sector and storing it.
type AttachSectorsData struct {
	TokenID  types.TokenID
	SectorID []byte
	// If true. keep the sector in the temporary store.
	// If false, the sector is moved from temporary store to the contract.
	KeepInTmp bool
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

	eventsQueue []interface{}

	closed bool
}

// NewTokenStorage - create new storage of tokens for prepaid downloads.
func NewTokenStorage(dir string) (*TokenStorage, error) {
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
	s := &TokenStorage{
		storage: storage,
		state:   state,
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

	if len(sectorIDs) == 0 {
		return nil
	}

	hasSectorIDs, err := t.state.HasSectors(id, sectorIDs)
	if err != nil {
		return fmt.Errorf("check has sector: %w", err)
	}
	if !hasSectorIDs {
		return fmt.Errorf("invalid request. one or more sectors don't exist")
	}

	t.applyEvent(&tokenstate.Event{EventRemoveSpecificSectors: &tokenstate.EventRemoveSpecificSectors{
		TokenID:    id,
		SectorsIDs: crypto.ConvertHashesToByteSlices(sectorIDs),
	}, Time: time})
	return nil
}

// RemoveAllSectors remove sectors from token.
func (t *TokenStorage) RemoveAllSectors(id types.TokenID, sectorsIDs []crypto.Hash, time time.Time) error {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.closed {
		return fmt.Errorf("token storage closed")
	}
	t.applyEvent(&tokenstate.Event{EventRemoveAllSectors: &tokenstate.EventRemoveAllSectors{
		TokenID:    id,
		SectorsIDs: crypto.ConvertHashesToByteSlices(sectorsIDs),
	}, Time: time})
	return nil
}

// AttachSectors attach sector to contract.
func (t *TokenStorage) AttachSectors(data []AttachSectorsData, time time.Time) error {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.closed {
		return fmt.Errorf("token storage closed")
	}
	var attachSectors []tokenstate.AttachSectorsData
	for _, el := range data {
		attachSectors = append(attachSectors, tokenstate.AttachSectorsData{
			TokenID:   el.TokenID,
			SectorID:  el.SectorID,
			KeepInTmp: el.KeepInTmp,
		})
	}
	t.applyEvent(&tokenstate.Event{EventAttachSectors: &tokenstate.EventAttachSectors{
		TokensSectors: attachSectors,
	}, Time: time})
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
func (t *TokenStorage) CheckExpiration(sm modules.StorageManager, frequency time.Duration, done chan bool) {
	ticker := time.NewTicker(frequency)
	for {
		select {
		case <-done:
			ticker.Stop()
			return

		case <-ticker.C:
			t.checkExpiration(sm)
		}
	}
}

func (t *TokenStorage) checkExpiration(sm modules.StorageManager) {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.closed {
		return
	}

	for token := range t.state.Tokens {
		if enough := t.state.EnoughStorageResource(token, 0, time.Now()); enough {
			continue
		}
		sectors, err := t.state.GetSectors(token)
		if err != nil {
			continue
		}

		t.applyEvent(&tokenstate.Event{EventRemoveAllSectors: &tokenstate.EventRemoveAllSectors{
			TokenID:    token,
			SectorsIDs: crypto.ConvertHashesToByteSlices(sectors),
		}, Time: time.Now()})

		// Removing a lot of sectors might take time, we can't do it under stateMu,
		// so we do it in a separate goroutine here. IDs of these sectors are
		// already removed from state, so there is no problem with returning
		// from this function before fully removing sectors data from disk.
		go func() {
			if err := sm.RemoveSectorBatch(sectors); err != nil {
				log.Printf("Failed to remove sectors of depleted token: %v", err)
			}
		}()
	}
}
