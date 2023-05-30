package renter

import (
	"encoding/json"
	"testing"
)

// TestWorkerAccountStatus is a small unit test that verifies the output of the
// `managedStatus` method on the worker's account.
func TestWorkerAccountStatus(t *testing.T) {
	t.Skip("EA workers disabled")
}

// TestWorkerPriceTableStatus is a small unit test that verifies the output of
// the worker's `staticPriceTableStatus` method.
func TestWorkerPriceTableStatus(t *testing.T) {
	t.Skip("EA workers disabled")
}

// TestWorkerReadJobStatus is a small unit test that verifies the output of the
// `callReadJobStatus` method on the worker.
func TestWorkerReadJobStatus(t *testing.T) {
	t.Skip("EA workers disabled")
}

// TestWorkerHasSectorJobStatus is a small unit test that verifies the output of
// the `callHasSectorJobStatus` method on the worker.
func TestWorkerHasSectorJobStatus(t *testing.T) {
	t.Skip("EA workers disabled")
}

// ToJSON is a helper function that wraps the jsonMarshalIndent function
func ToJSON(a interface{}) string {
	json, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(json)
}
