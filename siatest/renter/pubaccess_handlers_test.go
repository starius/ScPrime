package renter

import (
	"testing"

	"os"
	"path/filepath"
	"strings"

	"gitlab.com/scpcorp/ScPrime/node"
	"gitlab.com/scpcorp/ScPrime/persist"
	"gitlab.com/scpcorp/ScPrime/siatest"
	"gitlab.com/scpcorp/ScPrime/siatest/dependencies"
)

// TestSkynetSkylinkHandlerGET tests the behaviour of SkynetSkylinkHandlerGET
// when it handles different combinations of metadata and content. These tests
// use the fixtures in `testdata/publink_fixtures.json`.
func TestSkynetSkylinkHandlerGET(t *testing.T) {
	t.Skip()
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create a testgroup.
	groupParams := siatest.GroupParams{
		Hosts:  3,
		Miners: 1,
	}
	testDir := siatest.TestDir("renter", t.Name())
	if err := os.MkdirAll(testDir, persist.DefaultDiskPermissionsTest); err != nil {
		t.Fatal(err)
	}
	tg, err := siatest.NewGroupFromTemplate(testDir, groupParams)
	if err != nil {
		t.Fatal("Failed to create group: ", err)
	}
	defer func() {
		if err := tg.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	// Add a Renter node.
	renterParams := node.Renter(filepath.Join(testDir, "renter"))
	renterParams.RenterDeps = &dependencies.DependencyResolveSkylinkToFixture{}
	nodes, err := tg.AddNodes(renterParams)
	if err != nil {
		t.Fatal(err)
	}
	r := nodes[0]
	defer func() { _ = tg.RemoveNode(r) }()

	subTests := []struct {
		Name          string
		Publink       string
		ExpectedError string
	}{
		{
			// ValidSkyfile is the happy path, ensuring that we don't get errors
			// on valid data.
			Name:          "ValidSkyfile",
			Publink:       "_A6d-2CpM2OQ-7m5NPAYW830NdzC3wGydFzzd-KnHXhwJA",
			ExpectedError: "",
		},
		{
			// SingleFileDefaultPath ensures that we return an error if a single
			// file has a `defaultpath` field.
			Name:          "SingleFileDefaultPath",
			Publink:       "3AAcCO73xMbehYaK7bjDGCtW0GwOL6Swl-lNY52Pb_APzA",
			ExpectedError: "defaultpath is not allowed on single files",
		},
		{
			// DefaultPathDisableDefaultPath ensures that we return an error if
			// a file has both defaultPath and disableDefaultPath set.
			Name:          "DefaultPathDisableDefaultPath",
			Publink:       "3BBcCO73xMbehYaK7bjDGCtW0GwOL6Swl-lNY52Pb_APzA",
			ExpectedError: "both defaultpath and disabledefaultpath are set",
		},
		{
			// NonRootDefaultPath ensures that we return an error if a file has
			// both defaultPath and disableDefaultPath set.
			Name:          "NonRootDefaultPath",
			Publink:       "4BBcCO73xMbehYaK7bjDGCtW0GwOL6Swl-lNY52Pb_APzA",
			ExpectedError: "both defaultpath and disabledefaultpath are set",
		},
	}

	// Run the tests.
	for _, test := range subTests {
		r := tg.Renters()[0]
		_, _, err := r.SkynetPublinkGet(test.Publink)
		if err == nil && test.ExpectedError != "" {
			t.Fatalf("%s failed: %+v\n", test.Name, err)
		}
		if err != nil && (test.ExpectedError == "" || !strings.Contains(err.Error(), test.ExpectedError)) {
			t.Fatalf("%s failed: expected error '%s', got '%+v'\n", test.Name, test.ExpectedError, err)
		}
	}
}
