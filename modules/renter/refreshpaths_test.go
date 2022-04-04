package renter

import (
	"fmt"
	"testing"
	"time"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/renter/filesystem/siafile"
	"gitlab.com/scpcorp/ScPrime/persist"
	"gitlab.com/scpcorp/ScPrime/siatest/dependencies"

	"gitlab.com/NebulousLabs/fastrand"
)

// TestAddUniqueRefreshPaths probes the addUniqueRefreshPaths function
func TestAddUniqueRefreshPaths(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create a Renter
	rt, err := newRenterTesterWithDependency(t.Name(), &dependencies.DependencyDisableRepairAndHealthLoops{})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Create some directory tree paths
	paths := []modules.SiaPath{
		modules.RootSiaPath(),
		{Path: "root"},
		{Path: "root/SubDir1"},
		{Path: "root/SubDir1/SubDir1"},
		{Path: "root/SubDir1/SubDir1/SubDir1"},
		{Path: "root/SubDir1/SubDir2"},
		{Path: "root/SubDir2"},
		{Path: "root/SubDir2/SubDir1"},
		{Path: "root/SubDir2/SubDir2"},
		{Path: "root/SubDir2/SubDir2/SubDir2"},
	}

	// Create a map of directories to be refreshed
	dirsToRefresh := rt.renter.newUniqueRefreshPaths()

	// Add all paths to map
	for _, path := range paths {
		err = dirsToRefresh.callAdd(path)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Randomly add more paths
	for i := 0; i < 10; i++ {
		err = dirsToRefresh.callAdd(paths[fastrand.Intn(len(paths))])
		if err != nil {
			t.Fatal(err)
		}
	}

	// There should only be the following paths in the map
	uniquePaths := []modules.SiaPath{
		{Path: "root/SubDir1/SubDir1/SubDir1"},
		{Path: "root/SubDir1/SubDir2"},
		{Path: "root/SubDir2/SubDir1"},
		{Path: "root/SubDir2/SubDir2/SubDir2"},
	}
	if len(dirsToRefresh.childDirs) != len(uniquePaths) {
		t.Fatalf("Expected %v paths in map but got %v", len(uniquePaths), len(dirsToRefresh.childDirs))
	}
	for _, path := range uniquePaths {
		if _, ok := dirsToRefresh.childDirs[path]; !ok {
			t.Fatal("Did not find path in map", path)
		}
	}

	// Make child directories and add a file to each
	rsc, _ := siafile.NewRSCode(1, 1)
	up := modules.FileUploadParams{
		Source:      "",
		ErasureCode: rsc,
	}
	for _, sp := range uniquePaths {
		err = rt.renter.CreateDir(sp, modules.DefaultDirPerm)
		if err != nil {
			t.Fatal(err)
		}
		up.SiaPath, err = sp.Join("testFile")
		if err != nil {
			t.Fatal(err)
		}
		err = rt.renter.staticFileSystem.NewSiaFile(up.SiaPath, up.Source, up.ErasureCode, crypto.GenerateSiaKey(crypto.RandomCipherType()), 100, persist.DefaultDiskPermissionsTest, up.DisablePartialChunk)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Check the metadata of the root directory. Because we added files by
	// directly calling the staticFileSystem, a bubble should not have been
	// triggered and therefore the number of total files should be 0
	di, err := rt.renter.DirList(modules.RootSiaPath())
	if err != nil {
		t.Fatal(err)
	}
	if di[0].AggregateNumFiles != 0 {
		t.Fatal("Expected AggregateNumFiles to be 0 but got", di[0].AggregateNumFiles)
	}
	if di[0].AggregateNumSubDirs != 0 {
		t.Fatal("Expected AggregateNumSubDirs to be 0 but got", di[0].AggregateNumSubDirs)
	}

	// Have uniqueBubblePaths call bubble
	dirsToRefresh.callRefreshAll()

	// Wait for root directory to show proper number of files and subdirs.
	err = build.Retry(100, 100*time.Millisecond, func() error {
		di, err = rt.renter.DirList(modules.RootSiaPath())
		if err != nil {
			return err
		}
		if int(di[0].AggregateNumFiles) != len(dirsToRefresh.childDirs) {
			return fmt.Errorf("Expected AggregateNumFiles to be %v but got %v", len(dirsToRefresh.childDirs), di[0].AggregateNumFiles)
		}
		// Check that AggregateNumSubDirs equals the length of `paths`, minus
		// the root directory, plus the standard directories `home`,
		// `snapshots`, and `var`.
		numSubDirs := len(paths) - 1 + 2
		if int(di[0].AggregateNumSubDirs) != numSubDirs {
			return fmt.Errorf("Expected AggregateNumSubDirs to be %v but got %v", numSubDirs, di[0].AggregateNumSubDirs)
		}
		return nil
	})
	if err != nil {
		t.Log("Num dirs", len(di))
		t.Log("Directory Infos", di)
		t.Fatal(err)
	}
}
