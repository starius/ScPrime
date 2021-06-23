package tokenstate

import (
	"testing"

	"gitlab.com/zer0main/eventsourcing"
)

type stateForCmp struct {
	Tokens map[string]TokenRecord `json:"tokens"`
}

func encodeState(state *State) (result stateForCmp) {
	result.Tokens = make(map[string]TokenRecord, len(state.Tokens))

	for tokenID, tokenRecord := range state.Tokens {
		result.Tokens[tokenID.String()] = tokenRecord
	}
	return result
}

func TestState(t *testing.T) {
	stateGen := func() eventsourcing.StateLoader {
		return NewState()
	}
	encoderForCmp := func(state interface{}) interface{} {
		return encodeState(state.(*State))
	}
	eventsourcing.RunTests(t, stateGen, encoderForCmp)
}
