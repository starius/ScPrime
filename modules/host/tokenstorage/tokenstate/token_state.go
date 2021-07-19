package tokenstate

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
)

type tokenStorageInfo struct {
	Storage        uint64    `json:"storage"` // sectors * second
	LastChangeTime time.Time `json:"last_change_time"`
	SectorsNum     uint64    `json:"sectors_num"`
}

// TokenRecord include information about token record
type TokenRecord struct {
	DownloadBytes  int64            `json:"download_bytes"`
	UploadBytes    int64            `json:"upload_bytes"`
	SectorAccesses int64            `json:"sector_accesses"`
	TokenInfo      tokenStorageInfo `json:"token_info"`
}

// EventTopUp change of state when token replenishment
type EventTopUp struct {
	TokenID        types.TokenID   `json:"token_id"`
	ResourceType   types.Specifier `json:"resource_type"`
	ResourceAmount int64           `json:"resource_amount"`
}

// EventTokenDownload change of state when downloading
type EventTokenDownload struct {
	TokenID        types.TokenID `json:"token_id"`
	DownloadBytes  int64         `json:"download_bytes"`
	SectorAccesses int64         `json:"sector_accesses"`
}

// EventAddSectors represent adding sectors to token
type EventAddSectors struct {
	TokenID    types.TokenID `json:"token_id"`
	SectorsIDs [][]byte      `json:"sectors_ids"`
}

// EventRemoveSectors represent force removing sectors from token
type EventRemoveSectors struct {
	TokenID    types.TokenID `json:"token_id"`
	SectorsIDs [][]byte      `json:"sectors_ids"`
}

// EventAttachSectors represent attaching sectors to contract
type EventAttachSectors struct {
	TokenID          types.TokenID `json:"token_id"`
	SectorsIDs       [][]byte      `json:"sectors_ids"`
	IsDeletingNeeded bool          `json:"is_deleting_needed"`
}

// Event include state events
type Event struct {
	EventTopUp         *EventTopUp         `json:"event_top_up"`
	EventTokenDownload *EventTokenDownload `json:"event_token_download"`
	EventAddSectors    *EventAddSectors    `json:"event_add_sectors"`
	EventRemoveSectors *EventRemoveSectors `json:"event_remove_sectors"`
	EventAttachSectors *EventAttachSectors `json:"event_attach_sectors"`
	Time               time.Time           `json:"time"`
}

type sectorsDBer interface {
	Get(tokenID types.TokenID) ([]crypto.Hash, error)
	Put(tokenID types.TokenID, sectorID crypto.Hash) error
	Delete(tokenID types.TokenID, sectorID crypto.Hash) error
	BatchDelete(tokenID types.TokenID) error
	Close() error
}

// State representation of token storage state
type State struct {
	Tokens map[types.TokenID]TokenRecord `json:"tokens"`
	db     sectorsDBer
}

// NewState create new state
func NewState(dir string) (*State, error) {
	db, err := newSectorsDB(dir)
	if err != nil {
		return nil, err
	}
	return &State{
		Tokens: make(map[types.TokenID]TokenRecord),
		db:     db,
	}, nil
}

// Apply handle state events
func (s *State) Apply(e *Event) {
	applied := 0

	if e.EventTopUp != nil {
		s.eventTopUp(e.EventTopUp)
		applied++
	}
	if e.EventTokenDownload != nil {
		s.eventTokenDownload(e.EventTokenDownload)
		applied++
	}
	if e.EventAddSectors != nil {
		s.eventAddSectors(e.EventAddSectors)
		applied++
	}
	if e.EventRemoveSectors != nil {
		s.eventRemoveSectors(e.EventRemoveSectors)
		applied++
	}
	if e.EventAttachSectors != nil {
		s.eventAttachSectors(e.EventAttachSectors)
		applied++
	}
	if applied != 1 {
		panic(fmt.Sprintf("want 1 subevent, got %d", applied))
	}
}

func (s *State) eventTopUp(e *EventTopUp) {
	token := s.Tokens[e.TokenID]

	switch e.ResourceType {
	case modules.DownloadBytes:
		token.DownloadBytes += e.ResourceAmount
	case modules.UploadBytes:
		token.UploadBytes += e.ResourceAmount
	case modules.SectorAccesses:
		token.SectorAccesses += e.ResourceAmount
	case modules.Storage:
		token.TokenInfo.Storage += uint64(e.ResourceAmount)
	}
	s.Tokens[e.TokenID] = token
}

func (s *State) eventTokenDownload(e *EventTokenDownload) {
	token := s.Tokens[e.TokenID]
	token.DownloadBytes -= e.DownloadBytes
	token.SectorAccesses -= e.SectorAccesses
	s.Tokens[e.TokenID] = token
}

func (s *State) eventAddSectors(e *EventAddSectors) {
	token := s.Tokens[e.TokenID]
	token.TokenInfo.addSectors(uint64(len(e.SectorsIDs)))
	token.UploadBytes -= int64(len(e.SectorsIDs) * int(modules.SectorSize))
	s.Tokens[e.TokenID] = token

	for _, sec := range e.SectorsIDs {
		_ = s.db.Put(e.TokenID, crypto.ConvertBytesToHash(sec))
	}
}
func (s *State) eventRemoveSectors(e *EventRemoveSectors) {
	token := s.Tokens[e.TokenID]
	token.TokenInfo.removeSectors(uint64(len(e.SectorsIDs)))
	s.Tokens[e.TokenID] = token

	_ = s.db.BatchDelete(e.TokenID)
}

func (s *State) eventAttachSectors(e *EventAttachSectors) {
	var deleteSectorsNum uint64

	for _, sec := range e.SectorsIDs {
		if e.IsDeletingNeeded {
			_ = s.db.Delete(e.TokenID, crypto.ConvertBytesToHash(sec))
			deleteSectorsNum++
		}
	}
	token := s.Tokens[e.TokenID]
	token.TokenInfo.removeSectors(deleteSectorsNum)
	s.Tokens[e.TokenID] = token
}

// LoadHistory load history in state
func (s *State) LoadHistory(r io.Reader) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()

	for {
		event := &Event{}
		if err := decoder.Decode(event); err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		s.Apply(event)
	}
	return nil
}

// GetSectors return sectors IDs from database by token ID
func (s *State) GetSectors(tokenID types.TokenID) ([]crypto.Hash, error) {
	return s.db.Get(tokenID)
}

func (t *tokenStorageInfo) addSectors(sectorsNum uint64) {
	now := time.Now()
	stResource := uint64(now.Sub(t.LastChangeTime).Seconds()) * t.SectorsNum
	if t.Storage < stResource {
		t.Storage = 0
	} else {
		t.Storage = t.Storage - stResource
	}
	t.LastChangeTime = now
	t.SectorsNum += sectorsNum
}

func (t *tokenStorageInfo) removeSectors(sectorsNum uint64) {
	now := time.Now()
	stResource := uint64(now.Sub(t.LastChangeTime).Seconds()) * t.SectorsNum
	if t.Storage < stResource {
		t.Storage = 0
	} else {
		t.Storage = t.Storage - stResource
	}
	if sectorsNum > t.SectorsNum {
		t.SectorsNum = 0
	} else {
		t.SectorsNum -= sectorsNum
	}
	t.LastChangeTime = now
}

// HasStorage calculates the storage resource from the passed time
func (t *tokenStorageInfo) HasStorage(now time.Time) bool {
	return t.Storage > uint64(now.Sub(t.LastChangeTime).Seconds())*t.SectorsNum
}

// EnoughStorageResource checks if there is enough storage resource to store existing sectors and new ones
// for one second. if the resource is less, it will return false
func (s *State) EnoughStorageResource(id types.TokenID, sectorsNum uint64) (enoughResource bool) {
	token := s.Tokens[id]
	if token.TokenInfo.Storage < 1*(token.TokenInfo.SectorsNum+sectorsNum) {
		return false
	}
	return true
}

// Close close DB connection
func (s *State) Close() error {
	return s.db.Close()
}
