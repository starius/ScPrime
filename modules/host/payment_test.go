package host

import (
	"testing"

	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
)

// TestProcessPayment verifies the host's ProcessPayment method.
func TestProcessPayment(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// setup a host and renter pair with an emulated file contract between them
	pair, err := newRenterHostPair(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := pair.Close()
		if err != nil {
			t.Error(err)
		}
	}()
}

// addNoOpRevision is a helper method that adds a revision to the given
// obligation. In production this 'noOpRevision' is always added, however the
// obligation returned by `newTesterStorageObligation` does not add it.
func (ht *hostTester) addNoOpRevision(so storageObligation, renterPK types.SiaPublicKey) (storageObligation, error) {
	builder, err := ht.wallet.StartTransaction()
	if err != nil {
		return storageObligation{}, err
	}

	txnSet := so.OriginTransactionSet
	contractTxn := txnSet[len(txnSet)-1]
	fc := contractTxn.FileContracts[0]

	noOpRevision := types.FileContractRevision{
		ParentID: contractTxn.FileContractID(0),
		UnlockConditions: types.UnlockConditions{
			PublicKeys: []types.SiaPublicKey{
				renterPK,
				ht.host.publicKey,
			},
			SignaturesRequired: 2,
		},
		NewRevisionNumber:     fc.RevisionNumber + 1,
		NewFileSize:           fc.FileSize,
		NewFileMerkleRoot:     fc.FileMerkleRoot,
		NewWindowStart:        fc.WindowStart,
		NewWindowEnd:          fc.WindowEnd,
		NewValidProofOutputs:  fc.ValidProofOutputs,
		NewMissedProofOutputs: fc.MissedProofOutputs,
		NewUnlockHash:         fc.UnlockHash,
	}

	builder.AddFileContractRevision(noOpRevision)
	tSet, err := builder.Sign(true)
	if err != nil {
		return so, err
	}
	so.RevisionTransactionSet = tSet
	return so, nil
}

// TestRevisionFromRequest tests revisionFromRequest valid flow and some edge
// cases.
func TestRevisionFromRequest(t *testing.T) {
	recent := types.FileContractRevision{
		NewValidProofOutputs: []types.SiacoinOutput{
			{Value: types.SiacoinPrecision},
			{Value: types.SiacoinPrecision},
		},
		NewMissedProofOutputs: []types.SiacoinOutput{
			{Value: types.SiacoinPrecision},
			{Value: types.SiacoinPrecision},
			{Value: types.SiacoinPrecision},
		},
	}
	pbcr := modules.PayByContractRequest{
		NewValidProofValues: []types.Currency{
			types.SiacoinPrecision.Mul64(10),
			types.SiacoinPrecision.Mul64(100),
		},
		NewMissedProofValues: []types.Currency{
			types.SiacoinPrecision.Mul64(1000),
			types.SiacoinPrecision.Mul64(10000),
			types.SiacoinPrecision.Mul64(100000),
		},
	}

	// valid case
	rev := revisionFromRequest(recent, pbcr)
	if !rev.NewValidProofOutputs[0].Value.Equals(types.SiacoinPrecision.Mul64(10)) {
		t.Fatal("valid output 0 doesn't match")
	}
	if !rev.NewValidProofOutputs[1].Value.Equals(types.SiacoinPrecision.Mul64(100)) {
		t.Fatal("valid output 1 doesn't match")
	}
	if !rev.NewMissedProofOutputs[0].Value.Equals(types.SiacoinPrecision.Mul64(1000)) {
		t.Fatal("missed output 0 doesn't match")
	}
	if !rev.NewMissedProofOutputs[1].Value.Equals(types.SiacoinPrecision.Mul64(10000)) {
		t.Fatal("missed output 1 doesn't match")
	}
	if !rev.NewMissedProofOutputs[2].Value.Equals(types.SiacoinPrecision.Mul64(100000)) {
		t.Fatal("missed output 2 doesn't match")
	}

	// too few valid outputs
	pbcr = modules.PayByContractRequest{
		NewValidProofValues: []types.Currency{
			types.SiacoinPrecision.Mul64(10),
		},
		NewMissedProofValues: []types.Currency{
			types.SiacoinPrecision.Mul64(1000),
			types.SiacoinPrecision.Mul64(10000),
			types.SiacoinPrecision.Mul64(100000),
		},
	}
	_ = revisionFromRequest(recent, pbcr)

	// too many valid outputs
	pbcr = modules.PayByContractRequest{
		NewValidProofValues: []types.Currency{
			types.SiacoinPrecision.Mul64(10),
			types.SiacoinPrecision.Mul64(10),
			types.SiacoinPrecision.Mul64(10),
		},
		NewMissedProofValues: []types.Currency{
			types.SiacoinPrecision.Mul64(1000),
			types.SiacoinPrecision.Mul64(10000),
			types.SiacoinPrecision.Mul64(100000),
		},
	}
	_ = revisionFromRequest(recent, pbcr)

	// too few missed outputs.
	pbcr = modules.PayByContractRequest{
		NewValidProofValues: []types.Currency{
			types.SiacoinPrecision.Mul64(10),
			types.SiacoinPrecision.Mul64(100),
		},
		NewMissedProofValues: []types.Currency{
			types.SiacoinPrecision.Mul64(10000),
			types.SiacoinPrecision.Mul64(100000),
		},
	}
	_ = revisionFromRequest(recent, pbcr)

	// too many missed outputs.
	pbcr = modules.PayByContractRequest{
		NewValidProofValues: []types.Currency{
			types.SiacoinPrecision.Mul64(10),
			types.SiacoinPrecision.Mul64(100),
		},
		NewMissedProofValues: []types.Currency{
			types.SiacoinPrecision.Mul64(10000),
			types.SiacoinPrecision.Mul64(100000),
			types.SiacoinPrecision.Mul64(100000),
			types.SiacoinPrecision.Mul64(100000),
		},
	}
	_ = revisionFromRequest(recent, pbcr)
}
