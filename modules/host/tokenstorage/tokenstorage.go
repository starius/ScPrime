package tokenstorage

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"gitlab.com/scpcorp/ScPrime/modules/host/tokenstorage/tokenstate"
	"gitlab.com/scpcorp/ScPrime/types"
	"gitlab.com/zer0main/eventsourcing"
	"gitlab.com/zer0main/filestorage"
)

// TODO: the current version of tokens storage does not support reverting blocks.
// If contracts related to `TopUpToken` RPC are reverted, all the tokens resources remain in the storage.

const persistDelay = 1 * time.Second

// TokenRecord include information about token record
type TokenRecord struct {
	DownloadBytes  int64
	UploadBytes    int64
	SectorAccesses int64
}

type storage interface {
	Lock(ctx context.Context) error
	Unlock(ctx context.Context) error
	AppendMeta(ctx context.Context, callback func(w io.Writer) error) error
}

// TokenStorage - storage of tokens for prepaid downloads
type TokenStorage struct {
	storage storage
	state   *tokenstate.State
	stateMu sync.Mutex // For in-memory only.
	metaMu  sync.Mutex // For drainEventsQueue (involving IO).

	eventsQueue []interface{}

	closed bool
}

// NewTokenStorage - create new storage of tokens for prepaid downloads
func NewTokenStorage(dir string) (*TokenStorage, error) {
	storage := filestorage.NewFileStorage(dir)
	ctx := context.Background()

	if err := storage.Lock(ctx); err != nil {
		return nil, fmt.Errorf("failed to lock: %w", err)
	}

	var state *tokenstate.State
	err := storage.LoadMeta(ctx, func(r io.Reader) error {
		state = tokenstate.NewState()
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

// Close - drain event queue and close storage
func (t *TokenStorage) Close(ctx context.Context) error {
	t.stateMu.Lock()
	t.closed = true
	t.stateMu.Unlock()

	if err := t.drainEventsQueue(); err != nil {
		return fmt.Errorf("drainEventsQueue: %w", err)
	}
	if err := t.storage.Unlock(ctx); err != nil {
		return fmt.Errorf("failed to unlock: %w", err)
	}
	return nil
}

// drainEventsQueue writes all pending events to hard drive (append to file metadata.json).
// Mutex stateMu must NOT be locked when this function is called.
func (t *TokenStorage) drainEventsQueue() error {
	return eventsourcing.DrainEventsQueue(context.Background(), &t.stateMu, &t.metaMu, &t.eventsQueue, t.storage)
}

// TokenRecord return token record by id
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
	}, nil
}

// RecordDownload set token record fields
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
	}})
	return nil
}

// AddResources - add resource to token
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
	}})
	return nil
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
