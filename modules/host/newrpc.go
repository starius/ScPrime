package host

import (
	"encoding/json"
	"errors"
	"math"
	"math/bits"
	"sort"
	"sync/atomic"
	"time"

	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
	bolt "go.etcd.io/bbolt"
)

// managedRPCLoopSettings writes an RPC response containing the host's
// settings.
func (h *Host) managedRPCLoopSettings(s *rpcSession) error {
	atomic.AddUint64(&h.atomicSettingsCalls, 1)
	s.extendDeadline(modules.NegotiateSettingsTime)

	_, maxFee := h.tpool.FeeEstimation()
	h.mu.Lock()
	hes := h.externalSettings(maxFee)
	h.mu.Unlock()
	js, _ := json.Marshal(hes)
	resp := modules.LoopSettingsResponse{
		Settings: js,
	}
	if err := s.writeResponse(resp); err != nil {
		return err
	}
	return nil
}

// managedRPCLoopLock handles the LoopLock RPC.
func (h *Host) managedRPCLoopLock(s *rpcSession) error {
	s.extendDeadline(modules.NegotiateRecentRevisionTime)

	// Challenges can only be used once, so generate a new one immediately,
	// regardless of the outcome of this RPC.
	challenge := s.challenge
	fastrand.Read(s.challenge[:])

	// Read the request.
	var req modules.LoopLockRequest
	if err := s.readRequest(&req, modules.RPCMinLen); err != nil {
		s.writeError(err)
		return err
	}

	// Another contract may already be locked; locking multiple contracts is
	// not allowed.
	if len(s.so.OriginTransactionSet) != 0 {
		err := errors.New("another contract is already locked")
		s.writeError(err)
		return err
	}

	// Sanity-check the lock timeout
	lockTimeout := time.Duration(req.Timeout) * time.Millisecond
	if lockTimeout > maxObligationLockTimeout {
		err := errors.New("lock timeout is too long")
		s.writeError(err)
		return err
	}

	// look up the renter's public key
	var so storageObligation
	h.mu.RLock()
	err := h.db.View(func(tx *bolt.Tx) error {
		var err error
		so, err = h.getStorageObligation(tx, req.ContractID)
		return err
	})
	h.mu.RUnlock()
	if err != nil || h.dependencies.Disrupt("loopLockNoRecordOfThatContract") {
		s.writeError(errors.New(modules.V1420ContractNotRecognizedErrString))
		return extendErr("could not get storage obligation "+req.ContractID.String()+": ", err)
	}
	txn := so.RevisionTransactionSet[len(so.RevisionTransactionSet)-1]
	rev := txn.FileContractRevisions[0]
	var renterPK crypto.PublicKey
	copy(renterPK[:], rev.UnlockConditions.PublicKeys[0].Key)

	// verify the challenge response
	hash := crypto.HashAll(modules.RPCChallengePrefix, challenge)
	var renterSig crypto.Signature
	copy(renterSig[:], req.Signature)
	if crypto.VerifyHash(hash, renterPK, renterSig) != nil {
		err := errors.New("challenge signature is invalid")
		s.writeError(err)
		return err
	}

	// attempt to lock the storage obligation
	lockErr := h.managedTryLockStorageObligation(req.ContractID, lockTimeout)
	if lockErr == nil {
		// locking succeeded; set the session storage obligation
		//
		// NOTE: we have to get the obligation again because it may have changed
		// while we waited to acquire the lock
		h.mu.RLock()
		err = h.db.View(func(tx *bolt.Tx) error {
			var err error
			so, err = h.getStorageObligation(tx, req.ContractID)
			return err
		})
		h.mu.RUnlock()
		if err != nil {
			s.writeError(errors.New(modules.V1420ContractNotRecognizedErrString))
			return extendErr("could not get storage obligation "+req.ContractID.String()+": ", err)
		}
		s.so = so
	}

	// get the revision and signatures
	txn = so.RevisionTransactionSet[len(so.RevisionTransactionSet)-1]
	rev = txn.FileContractRevisions[0]
	var sigs []types.TransactionSignature
	for _, sig := range txn.TransactionSignatures {
		// The transaction may have additional signatures that are only
		// relevant to the host.
		if sig.ParentID == crypto.Hash(rev.ParentID) {
			sigs = append(sigs, sig)
		}
	}

	// Write the response.
	resp := modules.LoopLockResponse{
		Acquired:     lockErr == nil,
		NewChallenge: s.challenge,
		Revision:     rev,
		Signatures:   sigs,
	}
	if err := s.writeResponse(resp); err != nil {
		return err
	}
	return nil
}

// managedRPCLoopUnlock handles the LoopUnlock RPC. No response is sent.
func (h *Host) managedRPCLoopUnlock(s *rpcSession) error {
	s.extendDeadline(modules.NegotiateSettingsTime)
	if len(s.so.OriginTransactionSet) != 0 {
		h.managedUnlockStorageObligation(s.so.id())
		s.so = storageObligation{}
	}
	return nil
}

// managedRPCLoopWrite reads an upload request and responds with a signature
// for the new revision.
func (h *Host) managedRPCLoopWrite(s *rpcSession) error {
	s.extendDeadline(modules.NegotiateFileContractRevisionTime)
	// Read the request.
	var req modules.LoopWriteRequest
	if err := s.readRequest(&req, modules.SectorSize*5); err != nil {
		// Reading may have failed due to a closed connection; regardless, it
		// doesn't hurt to try and tell the renter about it.
		s.writeError(err)
		return err
	}
	// If no Merkle proof was requested, the renter's signature should be
	// sent immediately.
	var sigResponse modules.LoopWriteResponse
	if !req.MerkleProof {
		if err := s.readResponse(&sigResponse, modules.RPCMinLen); err != nil {
			return err
		}
	}

	// Check that a contract is locked.
	if len(s.so.OriginTransactionSet) == 0 {
		err := errors.New("no contract locked")
		s.writeError(err)
		return err
	}

	// Read some internal fields for later.
	_, maxFee := h.tpool.FeeEstimation()
	h.mu.Lock()
	blockHeight := h.blockHeight
	secretKey := h.secretKey
	settings := h.externalSettings(maxFee)
	h.mu.Unlock()
	currentRevision := s.so.RevisionTransactionSet[len(s.so.RevisionTransactionSet)-1].FileContractRevisions[0]

	// Process each action.
	newRoots := append([]crypto.Hash(nil), s.so.SectorRoots...)
	sectorsChanged := make(map[uint64]struct{}) // for construct Merkle proof
	var bandwidthRevenue types.Currency
	var sectorsRemoved []crypto.Hash
	sectorsGained := make(map[crypto.Hash][]byte)
	for _, action := range req.Actions {
		switch action.Type {
		case modules.WriteActionAppend:
			if uint64(len(action.Data)) != modules.SectorSize {
				s.writeError(ErrBadSectorSize)
				return ErrBadSectorSize
			}
			// Update sector roots.
			newRoot := crypto.MerkleRoot(action.Data)
			newRoots = append(newRoots, newRoot)
			sectorsGained[newRoot] = action.Data

			sectorsChanged[uint64(len(newRoots))-1] = struct{}{}

			// Update finances
			bandwidthRevenue = bandwidthRevenue.Add(settings.UploadBandwidthPrice.Mul64(modules.SectorSize))

		case modules.WriteActionTrim:
			numSectors := action.A
			if uint64(len(newRoots)) < numSectors {
				err := errors.New("trim size exceeds number of sectors")
				s.writeError(err)
				return err
			}
			// Update sector roots.
			sectorsRemoved = append(sectorsRemoved, newRoots[uint64(len(newRoots))-numSectors:]...)
			newRoots = newRoots[:uint64(len(newRoots))-numSectors]

			sectorsChanged[uint64(len(newRoots))] = struct{}{}

		case modules.WriteActionSwap:
			i, j := action.A, action.B
			if i >= uint64(len(newRoots)) || j >= uint64(len(newRoots)) {
				err := errors.New("illegal sector index")
				s.writeError(err)
				return err
			}
			// Update sector roots.
			newRoots[i], newRoots[j] = newRoots[j], newRoots[i]

			sectorsChanged[i] = struct{}{}
			sectorsChanged[j] = struct{}{}

		case modules.WriteActionUpdate:
			sectorIndex, offset := action.A, action.B
			if sectorIndex >= uint64(len(newRoots)) {
				err := errors.New("illegal sector index or offset")
				s.writeError(err)
				return err
			} else if offset+uint64(len(action.Data)) > modules.SectorSize {
				s.writeError(ErrIllegalOffsetAndLength)
				return ErrIllegalOffsetAndLength
			}
			// Update sector roots.
			sector, err := h.ReadSector(newRoots[sectorIndex])
			if err != nil {
				s.writeError(err)
				return err
			}
			copy(sector[offset:], action.Data)
			newRoot := crypto.MerkleRoot(sector)
			sectorsRemoved = append(sectorsRemoved, newRoots[sectorIndex])
			sectorsGained[newRoot] = sector
			newRoots[sectorIndex] = newRoot

			// Update finances.
			bandwidthRevenue = bandwidthRevenue.Add(settings.UploadBandwidthPrice.Mul64(uint64(len(action.Data))))

		default:
			err := errors.New("unknown action type " + action.Type.String())
			s.writeError(err)
			return err
		}
	}

	// Update finances.
	var storageRevenue, newCollateral types.Currency
	if len(newRoots) > len(s.so.SectorRoots) {
		bytesAdded := modules.SectorSize * uint64(len(newRoots)-len(s.so.SectorRoots))
		blocksRemaining := s.so.proofDeadline() - blockHeight
		blockBytesCurrency := types.NewCurrency64(uint64(blocksRemaining)).Mul64(bytesAdded)
		storageRevenue = settings.StoragePrice.Mul(blockBytesCurrency)
		newCollateral = newCollateral.Add(settings.Collateral.Mul(blockBytesCurrency))
	}

	// If a Merkle proof was requested, construct it.
	newMerkleRoot := cachedMerkleRoot(newRoots)
	var merkleResp modules.LoopWriteMerkleProof
	if req.MerkleProof {
		// Calculate which sectors changed.
		oldNumSectors := uint64(len(s.so.SectorRoots))
		proofRanges := make([]crypto.ProofRange, 0, len(sectorsChanged))
		for index := range sectorsChanged {
			if index < oldNumSectors {
				proofRanges = append(proofRanges, crypto.ProofRange{
					Start: index,
					End:   index + 1,
				})
			}
		}
		sort.Slice(proofRanges, func(i, j int) bool {
			return proofRanges[i].Start < proofRanges[j].Start
		})
		// Record old leaf hashes for all changed sectors.
		leafHashes := make([]crypto.Hash, len(proofRanges))
		for i, r := range proofRanges {
			leafHashes[i] = s.so.SectorRoots[r.Start]
		}
		// Construct the Merkle proof.
		merkleResp = modules.LoopWriteMerkleProof{
			OldSubtreeHashes: crypto.MerkleDiffProof(proofRanges, oldNumSectors, nil, s.so.SectorRoots),
			OldLeafHashes:    leafHashes,
			NewMerkleRoot:    newMerkleRoot,
		}
		// Calculate bandwidth cost of proof.
		proofSize := crypto.HashSize * (len(merkleResp.OldSubtreeHashes) + len(leafHashes) + 1)
		if proofSize < modules.RPCMinLen {
			proofSize = modules.RPCMinLen
		}
		bandwidthRevenue = bandwidthRevenue.Add(settings.DownloadBandwidthPrice.Mul64(uint64(proofSize)))
	}

	// construct the new revision
	newRevision := currentRevision
	newRevision.NewRevisionNumber = req.NewRevisionNumber
	for _, action := range req.Actions {
		if action.Type == modules.WriteActionAppend {
			newRevision.NewFileSize += modules.SectorSize
		} else if action.Type == modules.WriteActionTrim {
			newRevision.NewFileSize -= modules.SectorSize * action.A
		}
	}
	newRevision.NewFileMerkleRoot = newMerkleRoot
	newRevision.NewValidProofOutputs = make([]types.SiacoinOutput, len(currentRevision.NewValidProofOutputs))
	for i := range newRevision.NewValidProofOutputs {
		newRevision.NewValidProofOutputs[i] = types.SiacoinOutput{
			Value:      req.NewValidProofValues[i],
			UnlockHash: currentRevision.NewValidProofOutputs[i].UnlockHash,
		}
	}
	newRevision.NewMissedProofOutputs = make([]types.SiacoinOutput, len(currentRevision.NewMissedProofOutputs))
	for i := range newRevision.NewMissedProofOutputs {
		newRevision.NewMissedProofOutputs[i] = types.SiacoinOutput{
			Value:      req.NewMissedProofValues[i],
			UnlockHash: currentRevision.NewMissedProofOutputs[i].UnlockHash,
		}
	}

	// verify the new revision
	newRevenue := settings.BaseRPCPrice.Add(storageRevenue).Add(bandwidthRevenue)
	s.so.SectorRoots, newRoots = newRoots, s.so.SectorRoots // verifyRevision assumes new roots
	err := verifyRevision(s.so, newRevision, blockHeight, newRevenue, newCollateral)
	s.so.SectorRoots, newRoots = newRoots, s.so.SectorRoots
	if err != nil {
		s.writeError(err)
		return err
	}

	// If a Merkle proof was requested, send it and wait for the renter's signature.
	if req.MerkleProof {
		if err := s.writeResponse(merkleResp); err != nil {
			return err
		} else if err := s.readResponse(&sigResponse, modules.RPCMinLen); err != nil {
			return err
		}
	}

	// Sign the new revision.
	renterSig := types.TransactionSignature{
		ParentID:       crypto.Hash(newRevision.ParentID),
		CoveredFields:  types.CoveredFields{FileContractRevisions: []uint64{0}},
		PublicKeyIndex: 0,
		Signature:      sigResponse.Signature,
	}
	txn, err := createRevisionSignature(newRevision, renterSig, secretKey, blockHeight)
	if err != nil {
		s.writeError(err)
		return err
	}

	// Update the storage obligation.
	s.so.SectorRoots = newRoots
	s.so.PotentialStorageRevenue = s.so.PotentialStorageRevenue.Add(storageRevenue)
	s.so.RiskedCollateral = s.so.RiskedCollateral.Add(newCollateral)
	s.so.PotentialUploadRevenue = s.so.PotentialUploadRevenue.Add(bandwidthRevenue)
	s.so.RevisionTransactionSet = []types.Transaction{txn}
	err = h.managedModifyStorageObligation(s.so, sectorsRemoved, sectorsGained)
	if err != nil {
		s.writeError(err)
		return err
	}

	// Send the response.
	resp := modules.LoopWriteResponse{
		Signature: txn.TransactionSignatures[1].Signature,
	}
	if err := s.writeResponse(resp); err != nil {
		return err
	}
	return nil
}

// managedRPCLoopRead writes an RPC response containing the requested data
// (along with signatures and an optional Merkle proof).
func (h *Host) managedRPCLoopRead(s *rpcSession) error {
	s.extendDeadline(modules.NegotiateDownloadTime)

	// Read the request.
	var req modules.LoopReadRequest
	if err := s.readRequest(&req, modules.RPCMinLen); err != nil {
		// Reading may have failed due to a closed connection; regardless, it
		// doesn't hurt to try and tell the renter about it.
		s.writeError(err)
		return err
	}

	// As soon as we finish reading the request, we must begin listening for
	// RPCLoopReadStop, which may arrive at any time, and must arrive before the
	// RPC is considered complete.
	stopSignal := make(chan error, 1)
	startListeningForStopSignal(s, stopSignal, &modules.RPCLoopReadStop)

	// Check that a contract is locked.
	if err := checkContractLocked(s, stopSignal); err != nil {
		return err
	}

	// Read some internal fields for later.
	_, maxFee := h.tpool.FeeEstimation()
	h.mu.Lock()
	blockHeight := h.blockHeight
	secretKey := h.secretKey
	settings := h.externalSettings(maxFee)
	h.mu.Unlock()
	currentRevision := s.so.RevisionTransactionSet[len(s.so.RevisionTransactionSet)-1].FileContractRevisions[0]

	// Validate the request.
	if err := validateSections(s, req.MerkleProof, req.Sections); err != nil {
		return err
	}
	if err := validateProofValues(s, req.NewValidProofValues, req.NewMissedProofValues); err != nil {
		return err
	}

	// construct the new revision
	newRevision := buildNewRevision(currentRevision, req.NewRevisionNumber, req.NewValidProofValues, req.NewMissedProofValues)

	// calculate expected cost and verify against renter's revision
	totalCost := estimateCost(&settings, req.Sections)
	err := verifyPaymentRevision(currentRevision, newRevision, blockHeight, totalCost)
	if err != nil {
		s.writeError(err)
		return err
	}

	// Sign the new revision.
	renterSig := types.TransactionSignature{
		ParentID:       crypto.Hash(newRevision.ParentID),
		CoveredFields:  types.CoveredFields{FileContractRevisions: []uint64{0}},
		PublicKeyIndex: 0,
		Signature:      req.Signature,
	}
	txn, err := createRevisionSignature(newRevision, renterSig, secretKey, blockHeight)
	if err != nil {
		s.writeError(err)
		return err
	}
	hostSig := txn.TransactionSignatures[1].Signature

	// Update the storage obligation.
	paymentTransfer := currentRevision.ValidRenterPayout().Sub(newRevision.ValidRenterPayout())
	s.so.PotentialDownloadRevenue = s.so.PotentialDownloadRevenue.Add(paymentTransfer)
	s.so.RevisionTransactionSet = []types.Transaction{txn}
	err = h.managedModifyStorageObligation(s.so, nil, nil)
	if err != nil {
		s.writeError(err)
		return err
	}

	// enter response loop
	for i, sec := range req.Sections {
		// Fetch the requested data.
		sectorData, err := h.ReadSector(sec.MerkleRoot)
		if err != nil {
			s.writeError(err)
			return err
		}
		data := sectorData[sec.Offset : sec.Offset+sec.Length]

		// Construct the Merkle proof, if requested.
		var proof []crypto.Hash
		if req.MerkleProof {
			proofStart := int(sec.Offset) / crypto.SegmentSize
			proofEnd := int(sec.Offset+sec.Length) / crypto.SegmentSize
			proof = crypto.MerkleRangeProof(sectorData, proofStart, proofEnd)
		}

		// Send the response. If the renter sent a stop signal, or this is the
		// final response, include our signature in the response.
		resp := modules.LoopReadResponse{
			Signature:   nil,
			Data:        data,
			MerkleProof: proof,
		}
		select {
		case err := <-stopSignal:
			if err != nil {
				return err
			}
			resp.Signature = hostSig
			return s.writeResponse(resp)
		default:
		}
		if i == len(req.Sections)-1 {
			resp.Signature = hostSig
		}
		if err := s.writeResponse(resp); err != nil {
			return err
		}
	}
	// The stop signal must arrive before RPC is complete.
	return <-stopSignal
}

// managedRPCLoopFormContract handles the contract formation RPC.
func (h *Host) managedRPCLoopFormContract(s *rpcSession) error {
	// NOTE: this RPC contains two request/response exchanges.
	s.extendDeadline(modules.NegotiateFileContractTime)

	// Read the contract request.
	var req modules.LoopFormContractRequest
	if err := s.readRequest(&req, modules.TransactionSetSizeLimit); err != nil {
		s.writeError(err)
		return err
	}

	_, maxFee := h.tpool.FeeEstimation()
	h.mu.Lock()
	settings := h.externalSettings(maxFee)
	h.mu.Unlock()
	if !settings.AcceptingContracts {
		s.writeError(errors.New("host is not accepting new contracts"))
		return nil
	}

	// The host verifies that the file contract coming over the wire is
	// acceptable.
	txnSet := req.Transactions
	var renterPK crypto.PublicKey
	copy(renterPK[:], req.RenterKey.Key)
	if err := h.managedVerifyNewContract(txnSet, renterPK, settings); err != nil {
		s.writeError(err)
		return err
	}
	// The host adds collateral to the transaction.
	txnBuilder, newParents, newInputs, newOutputs, err := h.managedAddCollateral(settings, txnSet)
	if err != nil {
		s.writeError(err)
		return err
	}
	// Send any new inputs and outputs that were added to the transaction.
	resp := modules.LoopContractAdditions{
		Parents: newParents,
		Inputs:  newInputs,
		Outputs: newOutputs,
	}
	if err := s.writeResponse(resp); err != nil {
		return err
	}

	// The renter will now send transaction signatures for the file contract
	// transaction and a signature for the implicit no-op file contract
	// revision.
	var renterSigs modules.LoopContractSignatures
	if err := s.readResponse(&renterSigs, modules.RPCMinLen); err != nil {
		s.writeError(err)
		return err
	}

	// The host adds the renter transaction signatures, then signs the
	// transaction and submits it to the blockchain, creating a storage
	// obligation in the process.
	h.mu.RLock()
	hostCollateral := contractCollateral(settings, txnSet[len(txnSet)-1].FileContracts[0])
	h.mu.RUnlock()
	fca := finalizeContractArgs{
		builder:                 txnBuilder,
		renewal:                 false,
		renterPK:                renterPK,
		renterSignatures:        renterSigs.ContractSignatures,
		renterRevisionSignature: renterSigs.RevisionSignature,
		initialSectorRoots:      nil,
		hostCollateral:          hostCollateral,
		hostInitialRevenue:      types.ZeroCurrency,
		hostInitialRisk:         types.ZeroCurrency,
		settings:                settings,
	}
	hostTxnSignatures, hostRevisionSignature, newSOID, err := h.managedFinalizeContract(fca)
	if err != nil {
		s.writeError(err)
		return err
	}
	defer h.managedUnlockStorageObligation(newSOID)

	// Send our signatures for the contract transaction and initial revision.
	hostSigs := modules.LoopContractSignatures{
		ContractSignatures: hostTxnSignatures,
		RevisionSignature:  hostRevisionSignature,
	}
	if err := s.writeResponse(hostSigs); err != nil {
		return err
	}

	return nil
}

// managedRPCLoopRenewContract handles the LoopRenewContract RPC.
func (h *Host) managedRPCLoopRenewContract(s *rpcSession) error {
	// NOTE: this RPC contains two request/response exchanges.
	s.extendDeadline(modules.NegotiateRenewContractTime)

	// Read the renewal request.
	var req modules.LoopRenewContractRequest
	if err := s.readRequest(&req, modules.TransactionSetSizeLimit); err != nil {
		s.writeError(err)
		return err
	}
	_, max := h.tpool.FeeEstimation()
	h.mu.Lock()
	settings := h.externalSettings(max)
	h.mu.Unlock()
	if !settings.AcceptingContracts {
		s.writeError(errors.New("host is not accepting new contracts"))
		return nil
	} else if len(s.so.RevisionTransactionSet) == 0 {
		err := errors.New("no such contract")
		s.writeError(err)
		return err
	}

	// Verify that the transaction coming over the wire is a proper renewal.
	var renterPK crypto.PublicKey
	copy(renterPK[:], req.RenterKey.Key)
	err := h.managedVerifyRenewedContract(s.so, req.Transactions, renterPK)
	if err != nil {
		s.writeError(err)
		return extendErr("verification of renewal failed: ", err)
	}
	txnBuilder, newParents, newInputs, newOutputs, err := h.managedAddRenewCollateral(s.so, settings, req.Transactions)
	if err != nil {
		s.writeError(err)
		return extendErr("failed to add collateral: ", err)
	}
	// Send any new inputs and outputs that were added to the transaction.
	resp := modules.LoopContractAdditions{
		Parents: newParents,
		Inputs:  newInputs,
		Outputs: newOutputs,
	}
	if err := s.writeResponse(resp); err != nil {
		return err
	}

	// The renter will now send transaction signatures for the file contract
	// transaction and a signature for the implicit no-op file contract
	// revision.
	var renterSigs modules.LoopContractSignatures
	if err := s.readResponse(&renterSigs, modules.RPCMinLen); err != nil {
		s.writeError(err)
		return err
	}

	// The host adds the renter transaction signatures, then signs the
	// transaction and submits it to the blockchain, creating a storage
	// obligation in the process.
	h.mu.RLock()
	fc := req.Transactions[len(req.Transactions)-1].FileContracts[0]
	renewRevenue := renewBasePrice(s.so, settings, fc)
	renewRisk := renewBaseCollateral(s.so, settings, fc)
	renewCollateral, err := renewContractCollateral(s.so, settings, fc)
	h.mu.RUnlock()
	if err != nil {
		s.writeError(err)
		return extendErr("failed to compute contract collateral: ", err)
	}
	fca := finalizeContractArgs{
		builder:                 txnBuilder,
		renewal:                 false,
		renterPK:                renterPK,
		renterSignatures:        renterSigs.ContractSignatures,
		renterRevisionSignature: renterSigs.RevisionSignature,
		initialSectorRoots:      s.so.SectorRoots,
		hostCollateral:          renewCollateral,
		hostInitialRevenue:      renewRevenue,
		hostInitialRisk:         renewRisk,
		settings:                settings,
	}
	hostTxnSignatures, hostRevisionSignature, newSOID, err := h.managedFinalizeContract(fca)
	if err != nil {
		s.writeError(err)
		return extendErr("failed to finalize contract: ", err)
	}
	defer h.managedUnlockStorageObligation(newSOID)

	// Send our signatures for the contract transaction and initial revision.
	hostSigs := modules.LoopContractSignatures{
		ContractSignatures: hostTxnSignatures,
		RevisionSignature:  hostRevisionSignature,
	}
	if err := s.writeResponse(hostSigs); err != nil {
		return err
	}

	return nil
}

// managedRPCLoopSectorRoots writes an RPC response containing the requested
// contract roots (along with signatures and a Merkle proof).
func (h *Host) managedRPCLoopSectorRoots(s *rpcSession) error {
	s.extendDeadline(modules.NegotiateDownloadTime)

	// Read the request.
	var req modules.LoopSectorRootsRequest
	if err := s.readRequest(&req, modules.RPCMinLen); err != nil {
		// Reading may have failed due to a closed connection; regardless, it
		// doesn't hurt to try and tell the renter about it.
		s.writeError(err)
		return err
	}

	// Check that a contract is locked.
	if len(s.so.OriginTransactionSet) == 0 {
		err := errors.New("no contract locked")
		s.writeError(err)
		return err
	}

	// Read some internal fields for later.
	_, maxFee := h.tpool.FeeEstimation()
	h.mu.Lock()
	blockHeight := h.blockHeight
	secretKey := h.secretKey
	settings := h.externalSettings(maxFee)
	h.mu.Unlock()
	currentRevision := s.so.RevisionTransactionSet[len(s.so.RevisionTransactionSet)-1].FileContractRevisions[0]

	// Validate the request.
	var err error
	if req.NumRoots > settings.MaxDownloadBatchSize/crypto.HashSize {
		err = errLargeDownloadBatch
	}
	if req.RootOffset > uint64(len(s.so.SectorRoots)) || req.RootOffset+req.NumRoots > uint64(len(s.so.SectorRoots)) {
		err = errRequestOutOfBounds
	} else if len(req.NewValidProofValues) != len(currentRevision.NewValidProofOutputs) {
		err = errors.New("wrong number of valid proof values")
	} else if len(req.NewMissedProofValues) != len(currentRevision.NewMissedProofOutputs) {
		err = errors.New("wrong number of missed proof values")
	}
	if err != nil {
		s.writeError(err)
		return extendErr("download iteration request failed: ", err)
	}

	// Fetch the roots and construct the Merkle proof
	contractRoots := s.so.SectorRoots[req.RootOffset:][:req.NumRoots]
	proofStart := int(req.RootOffset)
	proofEnd := int(req.RootOffset + req.NumRoots)
	proof := crypto.MerkleSectorRangeProof(s.so.SectorRoots, proofStart, proofEnd)

	// construct the new revision
	newRevision := currentRevision
	newRevision.NewRevisionNumber = req.NewRevisionNumber
	newRevision.NewValidProofOutputs = make([]types.SiacoinOutput, len(currentRevision.NewValidProofOutputs))
	for i := range newRevision.NewValidProofOutputs {
		newRevision.NewValidProofOutputs[i] = types.SiacoinOutput{
			Value:      req.NewValidProofValues[i],
			UnlockHash: currentRevision.NewValidProofOutputs[i].UnlockHash,
		}
	}
	newRevision.NewMissedProofOutputs = make([]types.SiacoinOutput, len(currentRevision.NewMissedProofOutputs))
	for i := range newRevision.NewMissedProofOutputs {
		newRevision.NewMissedProofOutputs[i] = types.SiacoinOutput{
			Value:      req.NewMissedProofValues[i],
			UnlockHash: currentRevision.NewMissedProofOutputs[i].UnlockHash,
		}
	}

	// calculate expected cost and verify against renter's revision
	responseSize := (req.NumRoots + uint64(len(proof))) * crypto.HashSize
	if responseSize < modules.RPCMinLen {
		responseSize = modules.RPCMinLen
	}
	bandwidthCost := settings.DownloadBandwidthPrice.Mul64(responseSize)
	totalCost := settings.BaseRPCPrice.Add(bandwidthCost)
	err = verifyPaymentRevision(currentRevision, newRevision, blockHeight, totalCost)
	if err != nil {
		s.writeError(err)
		return extendErr("payment validation failed: ", err)
	}

	// Sign the new revision.
	renterSig := types.TransactionSignature{
		ParentID:       crypto.Hash(newRevision.ParentID),
		CoveredFields:  types.CoveredFields{FileContractRevisions: []uint64{0}},
		PublicKeyIndex: 0,
		Signature:      req.Signature,
	}
	txn, err := createRevisionSignature(newRevision, renterSig, secretKey, blockHeight)
	if err != nil {
		s.writeError(err)
		return extendErr("failed to create revision signature: ", err)
	}

	// Update the storage obligation.
	paymentTransfer := currentRevision.ValidRenterPayout().Sub(newRevision.ValidRenterPayout())
	s.so.PotentialDownloadRevenue = s.so.PotentialDownloadRevenue.Add(paymentTransfer)
	s.so.RevisionTransactionSet = []types.Transaction{txn}
	err = h.managedModifyStorageObligation(s.so, nil, nil)
	if err != nil {
		s.writeError(err)
		return extendErr("failed to modify storage obligation: ", err)
	}

	// send the response
	resp := modules.LoopSectorRootsResponse{
		Signature:   txn.TransactionSignatures[1].Signature,
		SectorRoots: contractRoots,
		MerkleProof: proof,
	}
	if err := s.writeResponse(resp); err != nil {
		return err
	}
	return nil
}

// managedRPCLoopRenewAndClearContract handles the LoopRenewClearContract RPC.
func (h *Host) managedRPCLoopRenewAndClearContract(s *rpcSession) error {
	// NOTE: this RPC contains two request/response exchanges.
	s.extendDeadline(modules.NegotiateRenewContractTime)

	// Read the renewal request.
	var req modules.LoopRenewAndClearContractRequest
	if err := s.readRequest(&req, modules.TransactionSetSizeLimit); err != nil {
		s.writeError(err)
		return err
	}

	// Read some internal fields for later.
	_, maxFee := h.tpool.FeeEstimation()
	h.mu.Lock()
	blockHeight := h.blockHeight
	secretKey := h.secretKey
	settings := h.externalSettings(maxFee)
	h.mu.Unlock()

	// Check that the old contract is locked.
	if len(s.so.OriginTransactionSet) == 0 {
		err := errors.New("no contract locked")
		s.writeError(err)
		return err
	}
	if !settings.AcceptingContracts {
		s.writeError(errors.New("host is not accepting new contracts"))
		return nil
	} else if len(s.so.RevisionTransactionSet) == 0 {
		err := errors.New("no such contract")
		s.writeError(err)
		return err
	}

	// Prepare the final revision of the old contract.
	currentRevision := s.so.RevisionTransactionSet[len(s.so.RevisionTransactionSet)-1].FileContractRevisions[0]
	newRevision := currentRevision
	newRevision.NewRevisionNumber = math.MaxUint64
	newRevision.NewFileMerkleRoot = crypto.Hash{}
	newRevision.NewFileSize = 0
	newRevision.NewValidProofOutputs = make([]types.SiacoinOutput, len(currentRevision.NewValidProofOutputs))
	for i := range newRevision.NewValidProofOutputs {
		newRevision.NewValidProofOutputs[i] = types.SiacoinOutput{
			Value:      req.FinalValidProofValues[i],
			UnlockHash: currentRevision.NewValidProofOutputs[i].UnlockHash,
		}
	}
	// The missed proof outputs become the valid ones since the host won't need
	// to provide a storage proof.
	newRevision.NewMissedProofOutputs = newRevision.NewValidProofOutputs

	// Verifiy the final revision of the old contract.
	newRevenue := settings.BaseRPCPrice
	newRoots := []crypto.Hash{}
	s.so.SectorRoots, newRoots = newRoots, s.so.SectorRoots // verifyRevision assumes new roots
	err := verifyClearingRevision(s.so, newRevision, blockHeight, newRevenue, types.ZeroCurrency)
	s.so.SectorRoots, newRoots = newRoots, s.so.SectorRoots
	if err != nil {
		s.writeError(err)
		return err
	}

	// Verify that the transaction coming over the wire is a proper renewal.
	var renterPK crypto.PublicKey
	copy(renterPK[:], req.RenterKey.Key)
	err = h.managedVerifyRenewedContract(s.so, req.Transactions, renterPK)
	if err != nil {
		s.writeError(err)
		return extendErr("verification of renewal failed: ", err)
	}
	txnBuilder, newParents, newInputs, newOutputs, err := h.managedAddRenewCollateral(s.so, settings, req.Transactions)
	if err != nil {
		s.writeError(err)
		return extendErr("failed to add collateral: ", err)
	}
	// Send any new inputs and outputs that were added to the transaction.
	resp := modules.LoopContractAdditions{
		Parents: newParents,
		Inputs:  newInputs,
		Outputs: newOutputs,
	}
	if err := s.writeResponse(resp); err != nil {
		return err
	}

	// The renter will now send transaction signatures for the file contract
	// transaction and a signature for the implicit no-op file contract
	// revision.
	var renterSigs modules.LoopRenewAndClearContractSignatures
	if err := s.readResponse(&renterSigs, modules.RPCMinLen); err != nil {
		s.writeError(err)
		return err
	}

	// Sign the new revision.
	renterSig := types.TransactionSignature{
		ParentID:       crypto.Hash(newRevision.ParentID),
		CoveredFields:  types.CoveredFields{FileContractRevisions: []uint64{0}},
		PublicKeyIndex: 0,
		Signature:      renterSigs.FinalRevisionSignature,
	}
	finalRevTxn, err := createRevisionSignature(newRevision, renterSig, secretKey, blockHeight)
	if err != nil {
		s.writeError(err)
		return err
	}

	// The host adds the renter transaction signatures, then signs the
	// transaction and submits it to the blockchain, creating a storage
	// obligation in the process.
	h.mu.RLock()
	fc := req.Transactions[len(req.Transactions)-1].FileContracts[0]
	renewRevenue := renewBasePrice(s.so, settings, fc)
	renewRisk := renewBaseCollateral(s.so, settings, fc)
	renewCollateral, err := renewContractCollateral(s.so, settings, fc)
	h.mu.RUnlock()
	if err != nil {
		s.writeError(err)
		return extendErr("failed to compute contract collateral: ", err)
	}
	fca := finalizeContractArgs{
		builder:                 txnBuilder,
		renewal:                 true,
		renterPK:                renterPK,
		renterSignatures:        renterSigs.ContractSignatures,
		renterRevisionSignature: renterSigs.RevisionSignature,
		initialSectorRoots:      s.so.SectorRoots,
		hostCollateral:          renewCollateral,
		hostInitialRevenue:      renewRevenue,
		hostInitialRisk:         renewRisk,
		settings:                settings,
	}
	hostTxnSignatures, hostRevisionSignature, newSOID, err := h.managedFinalizeContract(fca)
	if err != nil {
		s.writeError(err)
		return extendErr("failed to finalize contract: ", err)
	}
	defer h.managedUnlockStorageObligation(newSOID)

	// Clear the old storage obligatoin.
	s.so.SectorRoots = []crypto.Hash{}
	s.so.RevisionTransactionSet = []types.Transaction{finalRevTxn}
	// we don't count the sectors as being removed since we prevented
	// managedFinalizeContract from incrementing the counters on virtual sectors
	// before
	h.managedModifyStorageObligation(s.so, nil, nil)

	// Send our signatures for the contract transaction and initial revision.
	hostSigs := modules.LoopRenewAndClearContractSignatures{
		ContractSignatures: hostTxnSignatures,
		RevisionSignature:  hostRevisionSignature,

		FinalRevisionSignature: finalRevTxn.TransactionSignatures[1].Signature,
	}
	if err := s.writeResponse(hostSigs); err != nil {
		return err
	}

	return nil
}

func startListeningForStopSignal(s *rpcSession, ch chan error, spec *types.Specifier) {
	go func() {
		var id types.Specifier
		err := s.readResponse(&id, modules.RPCMinLen)
		if err != nil {
			ch <- err
		} else if id != *spec {
			ch <- errors.New("expected 'stop' from renter, got " + id.String())
		} else {
			ch <- nil
		}
	}()
}

func checkContractLocked(s *rpcSession, stopSig chan error) error {
	if len(s.so.OriginTransactionSet) == 0 {
		err := errors.New("no contract locked")
		s.writeError(err)
		<-stopSig
		return err
	}
	return nil
}

func validateSections(s *rpcSession, merkleProof bool, sections []modules.LoopReadRequestSection) error {
	for _, sec := range sections {
		var err error
		switch {
		case uint64(sec.Offset)+uint64(sec.Length) > modules.SectorSize:
			err = errRequestOutOfBounds
		case sec.Length == 0:
			err = errors.New("length cannot be zero")
		case merkleProof && (sec.Offset%crypto.SegmentSize != 0 || sec.Length%crypto.SegmentSize != 0):
			err = errors.New("offset and length must be multiples of SegmentSize when requesting a Merkle proof")
		}
		if err != nil {
			s.writeError(err)
			return err
		}
	}
	return nil
}

func validateProofValues(s *rpcSession, newValidProofValues, newMissedProofValues []types.Currency) error {
	currentRevision := s.so.RevisionTransactionSet[len(s.so.RevisionTransactionSet)-1].FileContractRevisions[0]
	// Validate the request.
	var err error
	switch {
	case len(newValidProofValues) != len(currentRevision.NewValidProofOutputs):
		err = errors.New("wrong number of valid proof values")
	case len(newMissedProofValues) != len(currentRevision.NewMissedProofOutputs):
		err = errors.New("wrong number of missed proof values")
	}
	if err != nil {
		s.writeError(err)
		return err
	}
	return nil
}

func buildNewRevision(
	currentRevision types.FileContractRevision,
	newRevisionNumber uint64,
	newValidProofValues,
	newMissedProofValues []types.Currency,
) types.FileContractRevision {
	newRevision := currentRevision
	newRevision.NewRevisionNumber = newRevisionNumber
	newRevision.NewValidProofOutputs = make([]types.SiacoinOutput, len(currentRevision.NewValidProofOutputs))
	for i := range newRevision.NewValidProofOutputs {
		newRevision.NewValidProofOutputs[i] = types.SiacoinOutput{
			Value:      newValidProofValues[i],
			UnlockHash: currentRevision.NewValidProofOutputs[i].UnlockHash,
		}
	}
	newRevision.NewMissedProofOutputs = make([]types.SiacoinOutput, len(currentRevision.NewMissedProofOutputs))
	for i := range newRevision.NewMissedProofOutputs {
		newRevision.NewMissedProofOutputs[i] = types.SiacoinOutput{
			Value:      newMissedProofValues[i],
			UnlockHash: currentRevision.NewMissedProofOutputs[i].UnlockHash,
		}
	}
	return newRevision
}

func estimateSectorsAccesses(sections []modules.LoopReadRequestSection) int64 {
	sectorAccesses := make(map[crypto.Hash]struct{})
	for _, sec := range sections {
		sectorAccesses[sec.MerkleRoot] = struct{}{}
	}
	return int64(len(sectorAccesses))
}

func estimateBandwidth(sections []modules.LoopReadRequestSection) int64 {
	var estBandwidth int64
	for _, sec := range sections {
		// use the worst-case proof size of 2*tree depth (this occurs when
		// proving across the two leaves in the center of the tree)
		estHashesPerProof := 2 * bits.Len64(modules.SectorSize/crypto.SegmentSize)
		estBandwidth += int64(sec.Length) + int64(estHashesPerProof*crypto.HashSize)
	}
	if estBandwidth < modules.RPCMinLen {
		estBandwidth = modules.RPCMinLen
	}
	return estBandwidth
}

func estimateCost(settings *modules.HostExternalSettings, sections []modules.LoopReadRequestSection) types.Currency {
	estBandwidth := estimateBandwidth(sections)
	sectorAccesses := estimateSectorsAccesses(sections)
	bandwidthCost := settings.DownloadBandwidthPrice.Mul64(uint64(estBandwidth))
	sectorAccessCost := settings.SectorAccessPrice.Mul64(uint64(sectorAccesses))
	totalCost := settings.BaseRPCPrice.Add(bandwidthCost).Add(sectorAccessCost)
	return totalCost
}

// managedRPCLoopTopUpToken creates/adds resources to the token specified
// in the request. It also creates new contract revision.
func (h *Host) managedRPCLoopTopUpToken(s *rpcSession) error {
	s.extendDeadline(modules.NegotiateSettingsTime)

	// Read the request.
	var req modules.LoopTopUpTokenRequest
	if err := s.readRequest(&req, modules.RPCMinLen); err != nil {
		// Reading may have failed due to a closed connection; regardless, it
		// doesn't hurt to try and tell the renter about it.
		s.writeError(err)
		return err
	}

	if len(s.so.OriginTransactionSet) == 0 {
		err := errors.New("no contract locked")
		s.writeError(err)
		return err
	}

	// Read some internal fields for later.
	h.mu.RLock()
	blockHeight := h.blockHeight
	secretKey := h.secretKey
	tokenStor := h.tokenStor
	h.mu.RUnlock()
	if tokenStor == nil {
		return errors.New("host module not loaded")
	}
	currentRevision := s.so.RevisionTransactionSet[len(s.so.RevisionTransactionSet)-1].FileContractRevisions[0]

	// Validate the request.
	if err := validateProofValues(s, req.NewValidProofValues, req.NewMissedProofValues); err != nil {
		return err
	}
	// TODO: support selling of token resources back to the host (i.e. make negative values for the following parameters sensible).
	// In such case, money flows from the host to the renter.
	// This is interesting concept, but before implementing it we have to consider all the possible outcomes, including speculating storage price.
	// This idea needs deeper economic comprehension.
	if req.ResourcesAmount <= 0 {
		return errors.New("negative resources amount is requested, but selling resources back is not yet implemented")
	}

	// Construct the new revision.
	newRevision := buildNewRevision(currentRevision, req.NewRevisionNumber, req.NewValidProofValues, req.NewMissedProofValues)

	// Calculate expected cost and verify against renter's revision.
	totalCost := h.calculatePriceByResource(req.ResourcesType, req.ResourcesAmount)
	err := verifyPaymentRevision(currentRevision, newRevision, blockHeight, totalCost)
	if err != nil {
		s.writeError(err)
		return err
	}

	// Sign the new revision.
	renterSig := types.TransactionSignature{
		ParentID:       crypto.Hash(newRevision.ParentID),
		CoveredFields:  types.CoveredFields{FileContractRevisions: []uint64{0}},
		PublicKeyIndex: 0,
		Signature:      req.Signature,
	}
	txn, err := createRevisionSignature(newRevision, renterSig, secretKey, blockHeight)
	if err != nil {
		s.writeError(err)
		return err
	}

	// Update the storage obligation.
	paymentTransfer := currentRevision.ValidRenterPayout().Sub(newRevision.ValidRenterPayout())
	switch req.ResourcesType {
	case modules.DownloadBytes:
		s.so.PotentialDownloadRevenue = s.so.PotentialDownloadRevenue.Add(paymentTransfer)
	case modules.SectorAccesses:
		s.so.PotentialDownloadRevenue = s.so.PotentialDownloadRevenue.Add(paymentTransfer)
	case modules.UploadBytes:
		s.so.PotentialUploadRevenue = s.so.PotentialUploadRevenue.Add(paymentTransfer)
	case modules.Storage:
		s.so.PotentialStorageRevenue = s.so.PotentialStorageRevenue.Add(paymentTransfer)
	}
	s.so.RevisionTransactionSet = []types.Transaction{txn}
	err = h.managedModifyStorageObligation(s.so, nil, nil)
	if err != nil {
		s.writeError(err)
		return err
	}

	// Save changes to token storage.
	id := types.TokenID(req.Token)
	if err := tokenStor.AddResources(id, req.ResourcesType, req.ResourcesAmount); err != nil {
		return err
	}

	// Send response.
	resp := modules.LoopTopUpTokenResponse{
		Signature: txn.TransactionSignatures[1].Signature,
	}
	if err := s.writeResponse(resp); err != nil {
		return err
	}
	return nil
}

// managedRPCLoopDownloadWithToken downloads data paying to the host using the token.
// Does not create new contract revision, just decreases the amount of resources on the token.
func (h *Host) managedRPCLoopDownloadWithToken(s *rpcSession) error {
	s.extendDeadline(modules.NegotiateDownloadTime)

	// Read the request.
	var req modules.LoopDownloadWithTokenRequest
	if err := s.readRequest(&req, modules.RPCMinLen); err != nil {
		// Reading may have failed due to a closed connection; regardless, it
		// doesn't hurt to try and tell the renter about it.
		s.writeError(err)
		return err
	}

	h.mu.RLock()
	tokenStor := h.tokenStor
	h.mu.RUnlock()
	if tokenStor == nil {
		return errors.New("host module not loaded")
	}

	// As soon as we finish reading the request, we must begin listening for
	// RPCLoopDownloadWithTokenStop, which may arrive at any time, and must arrive before the
	// RPC is considered complete.
	stopSignal := make(chan error, 1)
	startListeningForStopSignal(s, stopSignal, &modules.RPCLoopDownloadWithTokenStop)

	// Validate the request.
	if err := validateSections(s, req.MerkleProof, req.Sections); err != nil {
		return err
	}

	// Make sure token has enough resources to handle this RPC call.
	id := types.TokenID(req.Token)
	estBandwidth := estimateBandwidth(req.Sections)
	sectorAccesses := estimateSectorsAccesses(req.Sections)
	enoughBytes := true
	enoughSectors := true
	availableBandwidth := int64(0)
	availableSectors := int64(0)
	tokenResources, err := tokenStor.TokenRecord(id)
	if err == nil {
		// Token not found = no resources, and 0 is correct.
		availableBandwidth = tokenResources.DownloadBytes
		availableSectors = tokenResources.SectorAccesses
	}
	if availableBandwidth < estBandwidth {
		// Not enough download bandwidth.
		enoughBytes = false
	}
	if availableSectors < sectorAccesses {
		// Not enough sector accesses.
		enoughSectors = false
	}
	if !enoughBytes || !enoughSectors {
		// Send response indicating lack of resources.
		resp := modules.LoopDownloadWithTokenResponse{
			EnoughSectorAccesses: enoughSectors,
			EnoughBytes:          enoughBytes,
		}
		if err := s.writeResponse(resp); err != nil {
			return err
		}
		// The stop signal must arrive before RPC is complete.
		return <-stopSignal
	}
	if _, err := tokenStor.RecordDownload(id, estBandwidth, sectorAccesses, time.Now()); err != nil {
		h.log.Println(err)
	}

	// Enter response loop.
	for _, sec := range req.Sections {
		// Fetch the requested data.
		sectorData, err := h.ReadSector(sec.MerkleRoot)
		if err != nil {
			s.writeError(err)
			return err
		}
		data := sectorData[sec.Offset : sec.Offset+sec.Length]

		// Construct the Merkle proof, if requested.
		var proof []crypto.Hash
		if req.MerkleProof {
			proofStart := int(sec.Offset) / crypto.SegmentSize
			proofEnd := int(sec.Offset+sec.Length) / crypto.SegmentSize
			proof = crypto.MerkleRangeProof(sectorData, proofStart, proofEnd)
		}

		// Send the response.
		resp := modules.LoopDownloadWithTokenResponse{
			EnoughSectorAccesses: true,
			EnoughBytes:          true,
			Data:                 data,
			MerkleProof:          proof,
		}
		select {
		case err := <-stopSignal:
			if err != nil {
				return err
			}
			return s.writeResponse(resp)
		default:
		}
		if err := s.writeResponse(resp); err != nil {
			return err
		}
	}
	// The stop signal must arrive before RPC is complete.
	return <-stopSignal
}
