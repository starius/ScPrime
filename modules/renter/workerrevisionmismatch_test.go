package renter

import (
	"testing"
)

// TestRevisionSync is a unit test that verifies if the revision number fix is
// attempted and whether it properly resync the revision.
func TestRevisionSync(t *testing.T) {
	t.Skip("EA workers disabled")
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}
}

// TestSuspectRevisionMismatchFlag is a small unit test that verifes the methods
// involved in setting and unsetting the SuspectRevisionMismatch flag.
func TestSuspectRevisionMismatchFlag(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	wt, err := newWorkerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := wt.Close()
		if err != nil {
			t.Fatal(err)
		}
	}()

	// check whether flag is unset
	if wt.staticSuspectRevisionMismatch() {
		t.Fatal("Unexpected outcome")
	}

	// set the flag and verify that it's set
	wt.staticSetSuspectRevisionMismatch()
	if !wt.staticSuspectRevisionMismatch() {
		t.Fatal("Unexpected outcome")
	}

	// trigger the method that tries to fix the mismatch and verify it properly
	// unsets the flag
	wt.externTryFixRevisionMismatch()
	if wt.staticSuspectRevisionMismatch() {
		t.Fatal("Unexpected outcome")
	}
}
