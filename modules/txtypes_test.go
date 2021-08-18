// txtypes_test
package modules

import (
	"math"
	"testing"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/types"
)

// TestTXTypeTransactionType tests if transaction type is recognized correctly
func TestTXTypeTransactionType(t *testing.T) {
	// Create empty transaction
	// Transaction without inputs should be sorted as "Miner transaction", only miners are able to add them to a block.
	tx := &types.Transaction{}
	tt := TransactionType(tx)
	if tt != TXTypeMiner {
		t.Errorf("Empty transaction is TXTypeMiner, got %s", tt)
	}

	//Add some arbitrary data
	//Use a host announcement as arbitrary data
	sk, pk := crypto.GenerateKeyPair()
	spk := types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       pk[:],
	}
	announce, _ := CreateAnnouncement("localhost:43210", spk, sk)
	tx.ArbitraryData = [][]byte{announce}
	//Should still be miner tx since no siacoininputs
	if tt = TransactionType(tx); tt != TXTypeMiner {
		t.Errorf("Arbitrary data without any inputs is still %s tx, got %s", TXTypeMiner, tt)
	}

	//Add siafundinputs
	tx.SiafundInputs = []types.SiafundInput{{}}
	//Without minerfees it is considered to be Setup (create outputs for spending)
	if tt = TransactionType(tx); tt != TXTypeSetup {
		t.Errorf("Without minerfees even with siafundinputs it's expected to be %s tx, got %s", TXTypeSetup, tt)
	}

	//Add minerfees
	tx.MinerFees = []types.Currency{{}}
	if tt = TransactionType(tx); tt != TXTypeSPFMove {
		t.Errorf("With siafundinputs it's expected to be %s tx, got %s", TXTypeSPFMove, tt)
	}

	//Add siacoininputs
	tx.SiacoinInputs = []types.SiacoinInput{{}}
	if tt = TransactionType(tx); tt != TXTypeSPFMove {
		t.Errorf("SCP inputs present, SPF inputs present, so it should be %s tx, got %s", TXTypeSPFMove, tt)
	}

	//Remove SPF inputs and it should turn from SPF move into a host announcement since the arbitrary data is a valid announcement
	tx.SiafundInputs = make([]types.SiafundInput, 0)
	if tt = TransactionType(tx); tt != TXTypeHostAnnouncement {
		t.Errorf("Valid host announce with minerfees should be %s tx, got %s", TXTypeHostAnnouncement, tt)
	}

	//Invalidate host announcement data and it should turn from host announcement to arbitrary data tx
	tx.ArbitraryData = [][]byte{announce[:len(announce)-1]}
	if tt = TransactionType(tx); tt != TXTypeArbitraryData {
		t.Errorf("Random data in arbitrary data with minerfees should be %s tx, got %s", TXTypeArbitraryData, tt)
	}

	//Add siacoinoutputs and it should turn into SCP move tx regardless of arbitrary data
	tx.SiacoinOutputs = []types.SiacoinOutput{{}}
	if tt = TransactionType(tx); tt != TXTypeSCPMove {
		t.Errorf("Siacoininputs with minerfees and siacoinoutputs should be %s tx, got %s", TXTypeSCPMove, tt)
	}

	//Remove minerfees and it should turn into Setup tx again regardless of arbitrary data
	tx.MinerFees = make([]types.Currency, 0)
	if tt = TransactionType(tx); tt != TXTypeSetup {
		t.Errorf("Siacoininputs and siacoinoutputs without minerfees should be %s tx, got %s", TXTypeSetup, tt)
	}
	//Add back minerfees
	tx.MinerFees = []types.Currency{{}}

	//Add storageproof and it should turn into storage proof tx
	tx.StorageProofs = []types.StorageProof{{}}
	if tt = TransactionType(tx); tt != TXTypeStorageProof {
		t.Errorf("Expected %v tx, got %s", TXTypeStorageProof, tt)
	}
	tx.StorageProofs = make([]types.StorageProof, 0)

	//Add contract revision and it should turn into contract revision tx and
	// depending on revisionnumber and missedproofoutputs
	// determined if it's a regular revision or revision for contract renewal
	revision := types.FileContractRevision{}
	revision.NewRevisionNumber = 100
	tx.FileContractRevisions = []types.FileContractRevision{revision}
	if tt = TransactionType(tx); tt != TXTypeContractRevisionRegular {
		t.Errorf("Expected %v tx, got %s", TXTypeContractRevisionRegular, tt)
	}
	tx.FileContractRevisions[0].NewRevisionNumber = math.MaxUint64
	tx.FileContractRevisions[0].NewMissedProofOutputs = make([]types.SiacoinOutput, 2)
	if tt = TransactionType(tx); tt != TXTypeContractRevisionForRenew {
		t.Errorf("Expected %v tx, got %s", TXTypeContractRevisionForRenew, tt)
	}

	tx.FileContractRevisions[0].NewMissedProofOutputs = make([]types.SiacoinOutput, 3)
	if tt = TransactionType(tx); tt != TXTypeContractRevisionRegular {
		t.Errorf("Expected %v tx, got %s", TXTypeContractRevisionRegular, tt)
	}
	tx.FileContractRevisions = make([]types.FileContractRevision, 0)

	//Add contract and it should turn into form contract tx and
	// depending on merkleroot determined if it's a new contract or contract renewal
	contract := types.FileContract{}
	contract.RevisionNumber = 10
	//empty merkleroot = new contract
	contract.FileMerkleRoot = crypto.Hash{}
	tx.FileContracts = []types.FileContract{contract}
	if tt = TransactionType(tx); tt != TXTypeFileContractNew {
		t.Errorf("Expected %v tx, got %s", TXTypeFileContractNew, tt)
	}
	//empty merkleroot = new contract
	contract.FileMerkleRoot = crypto.HashBytes([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0})
	tx.FileContracts = []types.FileContract{contract}
	if tt = TransactionType(tx); tt != TXTypeFileContractRenew {
		t.Errorf("Expected %v tx, got %s", TXTypeFileContractRenew, tt)
	}

	//Add theoretically possible Revision to the contracts and should get Mixed
	tx.FileContractRevisions = []types.FileContractRevision{revision}
	if tt = TransactionType(tx); tt != TXTypeMixed {
		t.Errorf("Mixing contracts and revisions expected %v tx, got %s", TXTypeMixed, tt)
	}
}
