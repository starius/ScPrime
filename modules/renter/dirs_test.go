package renter

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/renter/filesystem"
	"gitlab.com/scpcorp/ScPrime/modules/renter/filesystem/siadir"
	"gitlab.com/scpcorp/ScPrime/siatest/dependencies"
)

// TestRenterCreateDirectories checks that the renter properly created metadata files
// for direcotries
func TestRenterCreateDirectories(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rt, err := newRenterTesterWithDependency(t.Name(), &dependencies.DependencyDisableRepairAndHealthLoops{})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Test creating directory
	siaPath, err := modules.NewSiaPath("foo/bar/baz")
	if err != nil {
		t.Fatal(err)
	}
	err = rt.renter.CreateDir(siaPath, modules.DefaultDirPerm)
	if err != nil {
		t.Fatal(err)
	}

	// Confirm that directory metadata files were created in all directories
	if err := rt.checkDirInitialized(modules.RootSiaPath()); err != nil {
		t.Fatal(err)
	}
	siaPath, err = modules.NewSiaPath("foo")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.checkDirInitialized(siaPath); err != nil {
		t.Fatal(err)
	}
	siaPath, err = modules.NewSiaPath("foo/bar")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.checkDirInitialized(siaPath); err != nil {
		t.Fatal(err)
	}
	siaPath, err = modules.NewSiaPath("foo/bar/baz")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.checkDirInitialized(siaPath); err != nil {
		t.Fatal(err)
	}
}

// checkDirInitialized is a helper function that checks that the directory was
// initialized correctly and the metadata file exist and contain the correct
// information
func (rt *renterTester) checkDirInitialized(siaPath modules.SiaPath) error {
	siaDir, err := rt.renter.staticFileSystem.OpenSiaDir(siaPath)
	if err != nil {
		return fmt.Errorf("unable to load directory %v metadata: %v", siaPath, err)
	}
	defer siaDir.Close()
	fullpath := siaPath.SiaDirMetadataSysPath(rt.renter.staticFileSystem.Root())
	if _, err := os.Stat(fullpath); err != nil {
		return err
	}

	// Check that metadata is default value
	metadata, err := siaDir.Metadata()
	if err != nil {
		return err
	}
	// Check Aggregate Fields
	if metadata.AggregateHealth != siadir.DefaultDirHealth {
		return fmt.Errorf("AggregateHealth not initialized properly: have %v expected %v", metadata.AggregateHealth, siadir.DefaultDirHealth)
	}
	if !metadata.AggregateLastHealthCheckTime.IsZero() {
		return fmt.Errorf("AggregateLastHealthCheckTime should be a zero timestamp: %v", metadata.AggregateLastHealthCheckTime)
	}
	if metadata.AggregateModTime.IsZero() {
		return fmt.Errorf("AggregateModTime not initialized: %v", metadata.AggregateModTime)
	}
	if metadata.AggregateMinRedundancy != siadir.DefaultDirRedundancy {
		return fmt.Errorf("AggregateMinRedundancy not initialized properly: have %v expected %v", metadata.AggregateMinRedundancy, siadir.DefaultDirRedundancy)
	}
	if metadata.AggregateStuckHealth != siadir.DefaultDirHealth {
		return fmt.Errorf("AggregateStuckHealth not initialized properly: have %v expected %v", metadata.AggregateStuckHealth, siadir.DefaultDirHealth)
	}
	// Check SiaDir Fields
	if metadata.Health != siadir.DefaultDirHealth {
		return fmt.Errorf("Health not initialized properly: have %v expected %v", metadata.Health, siadir.DefaultDirHealth)
	}
	if !metadata.LastHealthCheckTime.IsZero() {
		return fmt.Errorf("LastHealthCheckTime should be a zero timestamp: %v", metadata.LastHealthCheckTime)
	}
	if metadata.ModTime.IsZero() {
		return fmt.Errorf("ModTime not initialized: %v", metadata.ModTime)
	}
	if metadata.MinRedundancy != siadir.DefaultDirRedundancy {
		return fmt.Errorf("MinRedundancy not initialized properly: have %v expected %v", metadata.MinRedundancy, siadir.DefaultDirRedundancy)
	}
	if metadata.StuckHealth != siadir.DefaultDirHealth {
		return fmt.Errorf("StuckHealth not initialized properly: have %v expected %v", metadata.StuckHealth, siadir.DefaultDirHealth)
	}
	path, err := siaDir.Path()
	if err != nil {
		return err
	}
	if path != rt.renter.staticFileSystem.DirPath(siaPath) {
		return fmt.Errorf("Expected path to be %v, got %v", path, rt.renter.staticFileSystem.DirPath(siaPath))
	}
	return nil
}

// TestDirInfo probes the DirInfo method
func TestDirInfo(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rt, err := newRenterTesterWithDependency(t.Name(), &dependencies.DependencyDisableRepairAndHealthLoops{})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Create directory
	siaPath, err := modules.NewSiaPath("foo/")
	if err != nil {
		t.Fatal(err)
	}
	err = rt.renter.CreateDir(siaPath, modules.DefaultDirPerm)
	if err != nil {
		t.Fatal(err)
	}
	// Check that DirInfo returns the same information as stored in the metadata
	fooDirInfo, err := rt.renter.staticFileSystem.DirInfo(siaPath)
	if err != nil {
		t.Fatal(err)
	}
	rootDirInfo, err := rt.renter.staticFileSystem.DirInfo(modules.RootSiaPath())
	if err != nil {
		t.Fatal(err)
	}
	fooEntry, err := rt.renter.staticFileSystem.OpenSiaDir(siaPath)
	if err != nil {
		t.Fatal(err)
	}
	rootEntry, err := rt.renter.staticFileSystem.OpenSiaDir(modules.RootSiaPath())
	if err != nil {
		t.Fatal(err)
	}
	err = compareDirectoryInfoAndMetadata(fooDirInfo, fooEntry)
	if err != nil {
		t.Fatal(err)
	}
	err = compareDirectoryInfoAndMetadata(rootDirInfo, rootEntry)
	if err != nil {
		t.Fatal(err)
	}
}

// TestRenterListDirectory verifies that the renter properly lists the contents
// of a directory
func TestRenterListDirectory(t *testing.T) {
	t.Skip()
	if testing.Short() {
		t.SkipNow()
	}
	rt, err := newRenterTesterWithDependency(t.Name(), &dependencies.DependencyDisableRepairAndHealthLoops{})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Create directory
	siaPath, err := modules.NewSiaPath("foo/")
	if err != nil {
		t.Fatal(err)
	}
	err = rt.renter.CreateDir(siaPath, modules.DefaultDirPerm)
	if err != nil {
		t.Fatal(err)
	}

	// Upload a file
	_, err = rt.renter.newRenterTestFile()
	if err != nil {
		t.Fatal(err)
	}

	// Confirm that we get expected number of FileInfo and DirectoryInfo.
	directories, err := rt.renter.DirList(modules.RootSiaPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(directories) != 5 {
		t.Fatal("Expected 5 DirectoryInfos but got", len(directories))
	}
	files, err := rt.renter.FileList(modules.RootSiaPath(), false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatal("Expected 1 FileInfo but got", len(files))
	}

	// Refresh the directories.
	for _, dir := range directories {
		go rt.renter.callThreadedBubbleMetadata(dir.SiaPath)
	}

	// Wait for root directory to show proper number of files and subdirs.
	err = build.Retry(100, 100*time.Millisecond, func() error {
		directories, err = rt.renter.DirList(modules.RootSiaPath())
		if err != nil {
			return err
		}
		root := directories[0]
		// Check the aggregate and siadir fields.
		//
		// Expecting /home, /home/user, /var, /snapshots, /foo
		if root.AggregateNumSubDirs != 5 {
			return fmt.Errorf("Expected 5 subdirs in aggregate but got %v", root.AggregateNumSubDirs)
		}
		if root.NumSubDirs != 4 {
			return fmt.Errorf("Expected 4 subdirs but got %v", root.NumSubDirs)
		}
		if root.AggregateNumFiles != 1 {
			return fmt.Errorf("Expected 1 file in aggregate but got %v", root.AggregateNumFiles)
		}
		if root.NumFiles != 1 {
			return fmt.Errorf("Expected 1 file but got %v", root.NumFiles)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the directory information matches the on disk information
	rootDir, err := rt.renter.staticFileSystem.OpenSiaDir(modules.RootSiaPath())
	if err != nil {
		t.Fatal(err)
	}
	rootDir.Close()
	fooDir, err := rt.renter.staticFileSystem.OpenSiaDir(siaPath)
	if err != nil {
		t.Fatal(err)
	}
	homeDir, err := rt.renter.staticFileSystem.OpenSiaDir(modules.HomeFolder)
	if err != nil {
		t.Fatal(err)
	}
	snapshotsDir, err := rt.renter.staticFileSystem.OpenSiaDir(modules.BackupFolder)
	if err != nil {
		t.Fatal(err)
	}
	// Sort directories.
	sort.Slice(directories, func(i, j int) bool {
		return strings.Compare(directories[i].SiaPath.String(), directories[j].SiaPath.String()) < 0
	})
	if err = compareDirectoryInfoAndMetadata(directories[0], rootDir); err != nil {
		t.Error(err)
	}
	if err = compareDirectoryInfoAndMetadata(directories[1], fooDir); err != nil {
		t.Error(err)
	}
	if err = compareDirectoryInfoAndMetadata(directories[2], homeDir); err != nil {
		t.Error(err)
	}
	if err = compareDirectoryInfoAndMetadata(directories[3], snapshotsDir); err != nil {
		t.Error(err)
	}
}

// compareDirectoryInfoAndMetadata is a helper that compares the information in
// a DirectoryInfo struct and a SiaDirSetEntry struct
func compareDirectoryInfoAndMetadata(di modules.DirectoryInfo, siaDir *filesystem.DirNode) error {
	md, err := siaDir.Metadata()
	if err != nil {
		return err
	}

	// Compare Aggregate Fields
	if md.AggregateHealth != di.AggregateHealth {
		return fmt.Errorf("AggregateHealths not equal, %v and %v", md.AggregateHealth, di.AggregateHealth)
	}
	if di.AggregateLastHealthCheckTime != md.AggregateLastHealthCheckTime {
		return fmt.Errorf("AggregateLastHealthCheckTimes not equal %v and %v", di.AggregateLastHealthCheckTime, md.AggregateLastHealthCheckTime)
	}
	aggregateMaxHealth := math.Max(md.AggregateHealth, md.AggregateStuckHealth)
	if di.AggregateMaxHealth != aggregateMaxHealth {
		return fmt.Errorf("AggregateMaxHealths not equal %v and %v", di.AggregateMaxHealth, aggregateMaxHealth)
	}
	aggregateMaxHealthPercentage := modules.HealthPercentage(aggregateMaxHealth)
	if di.AggregateMaxHealthPercentage != aggregateMaxHealthPercentage {
		return fmt.Errorf("AggregateMaxHealthPercentage not equal %v and %v", di.AggregateMaxHealthPercentage, aggregateMaxHealthPercentage)
	}
	if md.AggregateMinRedundancy != di.AggregateMinRedundancy {
		return fmt.Errorf("AggregateMinRedundancy not equal, %v and %v", md.AggregateMinRedundancy, di.AggregateMinRedundancy)
	}
	if di.AggregateMostRecentModTime != md.AggregateModTime {
		return fmt.Errorf("AggregateModTimes not equal %v and %v", di.AggregateMostRecentModTime, md.AggregateModTime)
	}
	if md.AggregateNumFiles != di.AggregateNumFiles {
		return fmt.Errorf("AggregateNumFiles not equal, %v and %v", md.AggregateNumFiles, di.AggregateNumFiles)
	}
	if md.AggregateNumStuckChunks != di.AggregateNumStuckChunks {
		return fmt.Errorf("AggregateNumStuckChunks not equal, %v and %v", md.AggregateNumStuckChunks, di.AggregateNumStuckChunks)
	}
	if md.AggregateNumSubDirs != di.AggregateNumSubDirs {
		return fmt.Errorf("AggregateNumSubDirs not equal, %v and %v", md.AggregateNumSubDirs, di.AggregateNumSubDirs)
	}
	if md.AggregateSize != di.AggregateSize {
		return fmt.Errorf("AggregateSizes not equal, %v and %v", md.AggregateSize, di.AggregateSize)
	}
	if md.NumStuckChunks != di.AggregateNumStuckChunks {
		return fmt.Errorf("NumStuckChunks not equal, %v and %v", md.NumStuckChunks, di.AggregateNumStuckChunks)
	}
	// Compare Directory Fields
	if md.Health != di.Health {
		return fmt.Errorf("healths not equal, %v and %v", md.Health, di.Health)
	}
	if di.LastHealthCheckTime != md.LastHealthCheckTime {
		return fmt.Errorf("LastHealthCheckTimes not equal %v and %v", di.LastHealthCheckTime, md.LastHealthCheckTime)
	}
	maxHealth := math.Max(md.Health, md.StuckHealth)
	if di.MaxHealth != maxHealth {
		return fmt.Errorf("MaxHealths not equal %v and %v", di.MaxHealth, maxHealth)
	}
	maxHealthPercentage := modules.HealthPercentage(maxHealth)
	if di.MaxHealthPercentage != maxHealthPercentage {
		return fmt.Errorf("MaxHealthPercentage not equal %v and %v", di.MaxHealthPercentage, maxHealthPercentage)
	}
	if md.MinRedundancy != di.MinRedundancy {
		return fmt.Errorf("MinRedundancy not equal, %v and %v", md.MinRedundancy, di.MinRedundancy)
	}
	if di.MostRecentModTime != md.ModTime {
		return fmt.Errorf("ModTimes not equal %v and %v", di.MostRecentModTime, md.ModTime)
	}
	if md.NumFiles != di.NumFiles {
		return fmt.Errorf("NumFiles not equal, %v and %v", md.NumFiles, di.NumFiles)
	}
	if md.NumStuckChunks != di.NumStuckChunks {
		return fmt.Errorf("NumStuckChunks not equal, %v and %v", md.NumStuckChunks, di.NumStuckChunks)
	}
	if md.NumSubDirs != di.NumSubDirs {
		return fmt.Errorf("NumSubDirs not equal, %v and %v", md.NumSubDirs, di.NumSubDirs)
	}
	if md.Size != di.DirSize {
		return fmt.Errorf("Sizes not equal, %v and %v", md.Size, di.DirSize)
	}
	if md.StuckHealth != di.StuckHealth {
		return fmt.Errorf("stuck healths not equal, %v and %v", md.StuckHealth, di.StuckHealth)
	}
	return nil
}
