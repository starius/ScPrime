// txtypes
package modules

import (
	"math"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/types"
)

type TXType int

const (
	TXTypeSetup TXType = iota
	TXTypeMiner
	TXTypeSPFMove
	TXTypeFileContractNew
	TXTypeFileContractRenew
	TXTypeContractRevisionRegular
	TXTypeContractRevisionForRenew
	TXTypeStorageProof
	TXTypeHostAnnouncement
	TXTypeArbitraryData
	TXTypeSCPMove
	TXTypeMixed
)

func (t TXType) String() string {
	return [...]string{
		"Setup",
		"Miner",
		"SPF move",
		"New file contract",
		"Renew file contract",
		"Contract revision general",
		"Contract revision for renew",
		"Storage proof",
		"Host announcement",
		"Arbitrary data",
		"SCP move",
		"Composite",
	}[t]
}

// TransactionType returns the transaction type determined by the transaction
// contents. Sanity or validity of transaction is not checked as that is done
// by transactionpool when transactions get submitted
func TransactionType(t *types.Transaction) TXType {
	if len(t.SiacoinInputs)+len(t.SiafundInputs) == 0 {
		return TXTypeMiner
	}
	if len(t.MinerFees) == 0 {
		return TXTypeSetup
	}
	if len(t.FileContracts)+len(t.FileContractRevisions)+len(t.StorageProofs) > 1 {
		return TXTypeMixed
	}
	if len(t.SiafundInputs) > 0 {
		return TXTypeSPFMove
	}
	if len(t.FileContracts) > 0 {
		for _, contract := range t.FileContracts {
			if contract.FileMerkleRoot == (crypto.Hash{}) {
				return TXTypeFileContractNew
			}
		}
		return TXTypeFileContractRenew
	}
	if len(t.FileContractRevisions) > 0 {
		for _, revision := range t.FileContractRevisions {
			if revision.NewRevisionNumber == math.MaxUint64 {
				if len(revision.NewMissedProofOutputs) < 3 {
					return TXTypeContractRevisionForRenew
				}
			}
			return TXTypeContractRevisionRegular
		}
	}
	if len(t.StorageProofs) > 0 {
		return TXTypeStorageProof
	}
	if len(t.SiacoinOutputs) == 0 {
		for _, arb := range t.ArbitraryData {
			_, _, err := DecodeAnnouncement(arb)
			if err == nil {
				return TXTypeHostAnnouncement
			}
		}
		return TXTypeArbitraryData
	}
	return TXTypeSCPMove
}
