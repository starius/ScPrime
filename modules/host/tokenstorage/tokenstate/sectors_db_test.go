package tokenstate

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/types"
)

func TestSectorDB(t *testing.T) {
	dirName := "test-tmp"
	err := os.Mkdir(dirName, os.ModePerm)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err = os.RemoveAll(dirName); err != nil {
			t.Logf("remove all error: %v", err)
		}
	})

	db, err := newSectorsDB(dirName)
	t.Cleanup(func() {
		if err = db.Close(); err != nil {
			t.Logf("close database err: %v", err)
		}
	})
	require.NoError(t, err)

	// Fill database with sectors.
	token := types.TokenID{1}
	var originSectors []crypto.Hash
	for i := 0; i < 10; i++ {
		sector := crypto.HashObject(i)
		originSectors = append(originSectors, sector)
		err = db.Put(token, sector)
		require.NoError(t, err)
	}
	// Sort lexicographically as the db does.
	sort.Slice(originSectors, func(i, j int) bool {
		res := bytes.Compare(originSectors[i].Bytes(), originSectors[j].Bytes())
		return res == -1
	})

	t.Run("check has", func(t *testing.T) {
		ok, err := db.HasSectors(token, []crypto.Hash{originSectors[0]})
		require.NoError(t, err)
		require.True(t, ok)

		ok, err = db.HasSectors(token, []crypto.Hash{crypto.HashObject("nonexistent")})
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("get limited", func(t *testing.T) {
		cases := []struct {
			name             string
			inputPageID      string
			inputLimit       int
			outputSectors    []crypto.Hash
			outputNextPageID string
			error            bool
		}{
			{
				name:        "nonexistent page id",
				inputPageID: crypto.Hash{100}.String(), inputLimit: 1,
				error: true,
			},
			{
				name:        "get one first item",
				inputPageID: "", inputLimit: 1,
				outputSectors: originSectors[:1], outputNextPageID: originSectors[1].String(),
			},
			{
				name:        "get all items",
				inputPageID: "", inputLimit: 10,
				outputSectors: originSectors, outputNextPageID: "",
			},
			{
				name:        "get more items than exists",
				inputPageID: "", inputLimit: 11,
				outputSectors: originSectors, outputNextPageID: "",
			},
			{
				name:        "get single middle item",
				inputPageID: originSectors[1].String(), inputLimit: 1,
				outputSectors: originSectors[1:2], outputNextPageID: originSectors[2].String(),
			},
			{
				name:        "get several middle items",
				inputPageID: originSectors[1].String(), inputLimit: 5,
				outputSectors: originSectors[1:6], outputNextPageID: originSectors[6].String(),
			},
		}

		for _, c := range cases {
			got, nextPageID, err := db.GetLimited(token, c.inputPageID, c.inputLimit)
			if c.error {
				require.Error(t, err, c.name)
				continue
			}
			require.NoError(t, err, c.name)

			require.Equal(t, len(c.outputSectors), len(got))
			require.Equal(t, c.outputSectors, got, c.name)
			require.Equal(t, c.outputNextPageID, nextPageID, c.name)
		}
	})

	t.Run("get limited, load full", func(t *testing.T) {
		limits := []int{1, 2, 5, 6, 9, 10, 11}

		for _, limit := range limits {
			t.Run(fmt.Sprintf("limit %d", limit), func(t *testing.T) {
				gotSectors := []crypto.Hash{}
				pageID := ""
				for {
					sectors, nextPageID, err := db.GetLimited(token, pageID, limit)
					require.NoError(t, err)
					pageID = nextPageID

					gotSectors = append(gotSectors, sectors...)

					if nextPageID == "" {
						break
					}
				}
				require.Equal(t, originSectors, gotSectors)
			})
		}
	})

	t.Run("delete", func(t *testing.T) {
		err = db.BatchDeleteSpecific(token, originSectors[1:2])
		require.NoError(t, err)

		gotSectors, err := db.Get(token)
		require.NoError(t, err)
		require.Equal(t, len(originSectors)-1, len(gotSectors))
		require.Equal(t, append(originSectors[:1], originSectors[2:]...), gotSectors)

		err = db.BatchDeleteAll(token)
		require.NoError(t, err)
		gotSectors, err = db.Get(token)
		require.NoError(t, err)
		require.Equal(t, 0, len(gotSectors))
	})
}
