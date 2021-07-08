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

// TokenRecord include information about token record
type TokenRecord struct {
	DownloadBytes  int64 `json:"download_bytes"`
	UploadBytes    int64 `json:"upload_bytes"`
	SectorAccesses int64 `json:"sector_accesses"`
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
	SectorsIDs []crypto.Hash `json:"sectors_ids"`
	Time       time.Time     `json:"time"`
}

// EventRemoveSectors represent force removing sectors from token
type EventRemoveSectors struct {
	TokenID    types.TokenID `json:"token_id"`
	SectorsIDs []crypto.Hash `json:"sectors_ids"`
	Time       time.Time     `json:"time"`
}

// EventAttachSectors represent attaching sectors to contract
type EventAttachSectors struct {
	TokenID          types.TokenID `json:"token_id"`
	SectorsIDs       []crypto.Hash `json:"sectors_ids"`
	Time             time.Time     `json:"time"`
	IsDeletingNeeded bool          `json:"is_deleting_needed"`
}

// Event include state events
type Event struct {
	EventTopUp         *EventTopUp         `json:"event_top_up"`
	EventTokenDownload *EventTokenDownload `json:"event_token_download"`
	EventAddSectors    *EventAddSectors    `json:"event_add_sectors"`
	EventRemoveSectors *EventRemoveSectors `json:"event_remove_sectors"`
	EventAttachSectors *EventAttachSectors `json:"event_attach_sectors"`
}

// State representation of token storage state
type State struct {
	Tokens map[types.TokenID]TokenRecord `json:"tokens"`
	db     sectorsDBer
}

// NewState create new state
func NewState(dir string) *State {
	return &State{
		Tokens: make(map[types.TokenID]TokenRecord),
		db:     newSectorsDB(dir),
	}
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
	for _, sec := range e.SectorsIDs {
		_ = s.db.Put([]byte(e.TokenID.String()+sec.String()), nil)
	}
}
func (s *State) eventRemoveSectors(e *EventRemoveSectors) {
	_ = s.db.BatchDelete([]byte(e.TokenID.String()))
}

func (s *State) eventAttachSectors(e *EventAttachSectors) {
	for _, sec := range e.SectorsIDs {
		if e.IsDeletingNeeded {
			_ = s.db.Delete([]byte(e.TokenID.String() + sec.String()))
		} else {
			_ = s.db.Put([]byte(e.TokenID.String()+sec.String()), nil)
		}
	}
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
