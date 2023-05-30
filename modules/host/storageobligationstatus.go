package host

import (
	"strconv"
)

// The storage obligation status definitions
const (
	obligationUnresolved storageObligationStatus = iota // Indicatees that an unitialized value was used.
	obligationRejected                                  // Indicates that the obligation never got started, no revenue gained or lost.
	obligationSucceeded                                 // Indicates that the obligation was completed, revenues were gained.
	obligationFailed                                    // Indicates that the obligation failed, revenues and collateral were lost.
	obligationRenewed                                   // Indicates that the obligation was renewed.
)

// storageObligationStatus indicates the current status of a storage obligation
type storageObligationStatus uint64

// String converts a storageObligationStatus to a string.
func (i storageObligationStatus) String() string {
	if i == 0 {
		return "obligationUnresolved"
	}
	if i == 1 {
		return "obligationRejected"
	}
	if i == 2 {
		return "obligationSucceeded"
	}
	if i == 3 {
		return "obligationFailed"
	}
	if i == 4 {
		return "obligationRenewed"
	}
	return "storageObligationStatus(" + strconv.FormatInt(int64(i), 10) + ")"
}

// verifyStorageObligationStatus checks the storage obligation for presence in
// blockchain, expiration and potential renewal.
// The result should be usable to judge if it is safe to remove the data attached
// to the storage obligation (expired but not renewed) or should stay if it
// got renewed.
func verifyStorageObligationStatus(so storageObligation) storageObligationStatus {
	//TODO: do actual verification
	return so.ObligationStatus
}
