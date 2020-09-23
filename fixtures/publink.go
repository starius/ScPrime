package fixtures

import (
	"encoding/json"
	"errors"
	"io/ioutil"

	"gitlab.com/scpcorp/ScPrime/modules"
)

const (
	// Fixture paths:
	// These are relative paths to the fixtures data. They are relative to the
	// currently running test's home directory and do not depend on the location
	// of this implementation. This allows us to load different data for
	// different tests.

	// skylinkFixturesPath points to fixtures representing skylinks when they
	// are being downloaded. See the PublinkFixture struct.
	skylinkFixturesPath = "testdata/publink_fixtures.json"
)

type (
	// PublinkFixture holds the download representation of a Publink
	PublinkFixture struct {
		Metadata modules.PubfileMetadata `json:"metadata"`
		Content  []byte                  `json:"content"`
	}
)

// LoadPublinkFixture returns the PublinkFixture representation of a Publink.
//
// NOTES: Each test is run with its own directory as a working directory. This
// means that we can load a relative path and each test will load its own data
// or, at least, the data of its own directory.
func LoadPublinkFixture(link modules.Publink) (PublinkFixture, error) {
	b, err := ioutil.ReadFile(skylinkFixturesPath)
	if err != nil {
		return PublinkFixture{}, err
	}
	skylinkFixtures := make(map[string]PublinkFixture)
	err = json.Unmarshal(b, &skylinkFixtures)
	if err != nil {
		return PublinkFixture{}, err
	}
	fs, exists := skylinkFixtures[link.String()]
	if !exists {
		return PublinkFixture{}, errors.New("fixture not found")
	}
	return fs, nil
}
