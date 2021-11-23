package tokenstate

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
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

func createState(t *testing.T) *State {
	dirName := "test-tmp"
	require.NoError(t, os.Mkdir(dirName, os.ModePerm))
	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(dirName))
	})
	s, err := NewState(dirName)
	require.NoError(t, err)
	return s
}

func TestState(t *testing.T) {
	s := createState(t)
	stateGen := func() eventsourcing.StateLoader {
		return s
	}
	encoderForCmp := func(state interface{}) interface{} {
		return encodeState(state.(*State))
	}
	eventsourcing.RunTests(t, stateGen, encoderForCmp)
}

func TestState_EnoughStorageResource(t *testing.T) {
	s := createState(t)
	var token types.TokenID
	fastrand.Read(token[:])
	sectorsNum := int64(500)
	storageDuration := time.Minute
	// Setting weird time value because we need integer seconds.
	addSectorsTime := time.Date(2000, time.November, 1, 1, 0, 0, 0, time.UTC)
	s.Apply(&Event{
		EventTopUp: &EventTopUp{
			TokenID:        token,
			ResourceType:   modules.UploadBytes,
			ResourceAmount: int64(modules.SectorSize) * sectorsNum,
		},
		Time: addSectorsTime,
	})
	s.Apply(&Event{
		EventTopUp: &EventTopUp{
			TokenID:        token,
			ResourceType:   modules.Storage,
			ResourceAmount: sectorsNum * int64(storageDuration.Seconds()),
		},
		Time: addSectorsTime,
	})
	sectorIDs := make([]crypto.Hash, 0, int(sectorsNum))
	for i := 0; i < int(sectorsNum); i++ {
		var sectorID crypto.Hash
		fastrand.Read(sectorID[:])
		sectorIDs = append(sectorIDs, sectorID)
	}
	s.Apply(&Event{
		EventAddSectors: &EventAddSectors{
			TokenID:    token,
			SectorsIDs: crypto.ConvertHashesToByteSlices(sectorIDs),
		},
		Time: addSectorsTime,
	})
	newSectors := int64(1)
	require.True(t, s.EnoughStorageResource(token, newSectors, addSectorsTime.Add(storageDuration).Add(-2*time.Second)))
	// We topped up for exactly storageDuration time. EnoughStorageResource must return false, since we have no resources left for extra sectors.
	require.False(t, s.EnoughStorageResource(token, newSectors, addSectorsTime.Add(storageDuration).Add(-time.Second)))
}
