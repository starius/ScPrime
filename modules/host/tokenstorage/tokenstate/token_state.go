package tokenstate

import (
	"encoding/json"
	"fmt"
	"io"

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

// Event include state events
type Event struct {
	EventTopUp         *EventTopUp         `json:"event_top_up"`
	EventTokenDownload *EventTokenDownload `json:"event_token_download"`
}

// State representation of token storage state
type State struct {
	Tokens map[types.TokenID]TokenRecord `json:"tokens"`
}

// NewState create new state
func NewState() *State {
	return &State{
		Tokens: make(map[types.TokenID]TokenRecord),
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
