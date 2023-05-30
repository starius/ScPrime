package renter

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/siatest/dependencies"
	"gitlab.com/scpcorp/ScPrime/types"
)

// workerTester is a helper type which contains a renter, host and worker that
// communicates with that host.
type workerTester struct {
	rt   *renterTester
	host modules.Host
	*worker
}

// newWorkerTester creates a new worker for testing.
func newWorkerTester(name string) (*workerTester, error) {
	return newWorkerTesterCustomDependency(name, modules.ProdDependencies, modules.ProdDependencies)
}

// newWorkerTesterCustomDependency creates a new worker for testing with a
// custom depency.
func newWorkerTesterCustomDependency(name string, renterDeps modules.Dependencies, hostDeps modules.Dependencies) (*workerTester, error) {
	// Create the renter.
	rt, err := newRenterTesterWithDependency(filepath.Join(name, "renter"), renterDeps)
	if err != nil {
		return nil, err
	}

	// Set an allowance.
	err = rt.renter.hostContractor.SetAllowance(modules.DefaultAllowance)
	if err != nil {
		return nil, err
	}

	// Add a host.
	host, err := rt.addCustomHost(filepath.Join(rt.dir, "host"), hostDeps)
	if err != nil {
		return nil, err
	}

	// Wait for worker to show up.
	var w *worker
	err = build.Retry(100, 100*time.Millisecond, func() error {
		_, err := rt.miner.AddBlock()
		if err != nil {
			return err
		}
		rt.renter.staticWorkerPool.callUpdate()
		workers := rt.renter.staticWorkerPool.callWorkers()
		if len(workers) != 1 {
			return fmt.Errorf("expected %v workers but got %v", 1, len(workers))
		}
		w = workers[0]
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &workerTester{
		rt:     rt,
		host:   host,
		worker: w,
	}, nil
}

// Close closes the renter and host.
func (wt *workerTester) Close() error {
	err1 := wt.rt.Close()
	err2 := wt.host.Close()
	return errors.Compose(err1, err2)
}

// TestNewWorkerTester creates a new worker
func TestNewWorkerTester(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	wt, err := newWorkerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	if err := wt.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestReadOffsetCorruptProof tests that ReadOffset jobs correctly verify the
// merkle proof returned by the host and reject data that doesn't match said
// proof.
func TestReadOffsetCorruptedProof(t *testing.T) {
	t.Skip("EA workers disabled")
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	deps := dependencies.NewDependencyCorruptMDMOutput()
	wt, err := newWorkerTesterCustomDependency(t.Name(), modules.ProdDependencies, deps)
	if err != nil {
		t.Errorf("newWorkerTesterCustomDependency: %v", err.Error())
	}
	defer func() {
		if err := wt.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	backup := modules.UploadedBackup{
		Name:           "foo",
		CreationDate:   types.CurrentTimestamp(),
		Size:           10,
		UploadProgress: 0,
	}

	// Upload a snapshot to fill the first sector of the contract.
	err = wt.UploadSnapshot(context.Background(), backup, fastrand.Bytes(int(backup.Size)))
	if err != nil {
		t.Errorf("UploadSnapshot failed: %v", err.Error())
	}

	// Download the first sector partially and then fully since both actions
	// require different proofs.
	_, err = wt.ReadOffset(context.Background(), 0, modules.SectorSize/2)
	if err != nil {
		t.Errorf("Half sector read offset failed: %v", err.Error())
	}
	_, err = wt.ReadOffset(context.Background(), 0, modules.SectorSize)
	if err != nil {
		t.Errorf("read offset full sector failed: %v", err.Error())
	}

	// Do it again but this time corrupt the output to make sure the proof
	// doesn't match.
	deps.Fail()
	_, err = wt.ReadOffset(context.Background(), 0, modules.SectorSize/2)
	if err == nil || !strings.Contains(err.Error(), "verifying proof failed") {
		t.Fatal(err)
	}

	// Retry since the worker might be on a cooldown.
	err = build.Retry(100, 100*time.Millisecond, func() error {
		deps.Fail()
		_, err = wt.ReadOffset(context.Background(), 0, modules.SectorSize)
		if err == nil || !strings.Contains(err.Error(), "verifying proof failed") {
			return fmt.Errorf("unexpected error %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
