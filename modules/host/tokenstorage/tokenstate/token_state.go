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
	Storage        int64     `json:"storage"` // sectors * second
	LastChangeTime time.Time `json:"last_change_time"`
	SectorsNum     uint64    `json:"sectors_num"`
}

// TokenRecord include information about token record.
type TokenRecord struct {
	DownloadBytes  int64            `json:"download_bytes"`
	UploadBytes    int64            `json:"upload_bytes"`
	SectorAccesses int64            `json:"sector_accesses"`
	TokenInfo      tokenStorageInfo `json:"token_info"`
}

// AttachSectorsData include information about token sector and storing it.
type AttachSectorsData struct {
	TokenID  types.TokenID
	SectorID []byte
	// If true. keep the sector in the temporary store.
	// If false, the sector is moved from temporary store to the contract.
	KeepInTmp bool
}

// EventTopUp change of state when token replenishment.
type EventTopUp struct {
	TokenID        types.TokenID   `json:"token_id"`
	ResourceType   types.Specifier `json:"resource_type"`
	ResourceAmount int64           `json:"resource_amount"`
}

// EventTokenDownload change of state when downloading.
type EventTokenDownload struct {
	TokenID        types.TokenID `json:"token_id"`
	DownloadBytes  int64         `json:"download_bytes"`
	SectorAccesses int64         `json:"sector_accesses"`
}

// EventAddSectors represent adding sectors to token.
type EventAddSectors struct {
	TokenID    types.TokenID `json:"token_id"`
	SectorsIDs [][]byte      `json:"sectors_ids"`
}

// EventRemoveSectors represent force removing sectors from token.
type EventRemoveSectors struct {
	TokenID    types.TokenID `json:"token_id"`
	SectorsIDs [][]byte      `json:"sectors_ids"`
}

// EventAttachSectors represent attaching sector to contract.
type EventAttachSectors struct {
	TokensSectors []AttachSectorsData `json:"tokens_sectors"`
}

// Event include state events.
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

// State representation of token storage state.
type State struct {
	Tokens map[types.TokenID]TokenRecord `json:"tokens"`
	db     sectorsDBer
}

// NewState create new state.
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

// Apply handle state events.
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
		s.eventAddSectors(e.EventAddSectors, e.Time)
		applied++
	}
	if e.EventRemoveSectors != nil {
		s.eventRemoveSectors(e.EventRemoveSectors, e.Time)
		applied++
	}
	if e.EventAttachSectors != nil {
		s.eventAttachSectors(e.EventAttachSectors, e.Time)
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
		token.TokenInfo.Storage += e.ResourceAmount
	}
	s.Tokens[e.TokenID] = token
}

func (s *State) eventTokenDownload(e *EventTokenDownload) {
	token := s.Tokens[e.TokenID]
	token.DownloadBytes -= e.DownloadBytes
	token.SectorAccesses -= e.SectorAccesses
	s.Tokens[e.TokenID] = token
}

func (s *State) eventAddSectors(e *EventAddSectors, t time.Time) {
	token := s.Tokens[e.TokenID]
	token.TokenInfo.updateStorageResource(int64(len(e.SectorsIDs)), t)
	token.UploadBytes -= int64(len(e.SectorsIDs) * int(modules.SectorSize))
	s.Tokens[e.TokenID] = token

	for _, sec := range e.SectorsIDs {
		err := s.db.Put(e.TokenID, crypto.ConvertBytesToHash(sec))
		if err != nil {
			panic(err)
		}
	}
}
func (s *State) eventRemoveSectors(e *EventRemoveSectors, t time.Time) {
	token := s.Tokens[e.TokenID]
	token.TokenInfo.updateStorageResource(-int64(len(e.SectorsIDs)), t)
	s.Tokens[e.TokenID] = token
	err := s.db.BatchDelete(e.TokenID)
	if err != nil {
		panic(err)
	}
}

func (s *State) eventAttachSectors(e *EventAttachSectors, t time.Time) {
	for _, ts := range e.TokensSectors {
		if !ts.KeepInTmp {
			err := s.db.Delete(ts.TokenID, crypto.HashBytes(ts.SectorID))
			if err != nil {
				panic(err)
			}
		}
		token := s.Tokens[ts.TokenID]
		token.TokenInfo.updateStorageResource(-1, t)
		s.Tokens[ts.TokenID] = token
	}
}

// LoadHistory load history in state.
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

// GetSectors return sectors IDs from database by token ID.
func (s *State) GetSectors(tokenID types.TokenID) ([]crypto.Hash, error) {
	return s.db.Get(tokenID)
}

func (t *tokenStorageInfo) updateStorageResource(sectorsNum int64, now time.Time) {
	stResource := int64(now.Sub(t.LastChangeTime).Seconds()) * int64(t.SectorsNum)
	t.Storage = t.Storage - stResource
	// sectorsNum can be negative for removing sectors.
	if sectorsNum < 0 {
		if uint64(-sectorsNum) > t.SectorsNum {
			t.SectorsNum = 0
		} else {
			t.SectorsNum -= uint64(-sectorsNum)
		}
	} else {
		t.SectorsNum += uint64(sectorsNum)
	}
	t.LastChangeTime = now
}

// EnoughStorageResource checks if there is enough storage resource to store existing sectors and new ones
// for one second. if the resource is less, it will return false.
func (s *State) EnoughStorageResource(id types.TokenID, sectorsNum int64, now time.Time) (enoughResource bool) {
	token := s.Tokens[id]
	if token.TokenInfo.Storage > int64(now.Sub(token.TokenInfo.LastChangeTime).Seconds())*int64(token.TokenInfo.SectorsNum)+sectorsNum*1 {
		return true
	}
	return false
}

// Close DB connection.
func (s *State) Close() error {
	return s.db.Close()
}
