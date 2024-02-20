package wallet

import (
	"context"
	"fmt"
	"time"

	"gitlab.com/scpcorp/ScPrime/types"
	"gitlab.com/scpcorp/spf-transporter"
	"gitlab.com/scpcorp/spf-transporter/common"
)

// SpfBurntAmount returns amount of SPF burnt in this transaction.
func SpfBurntAmount(tx *types.Transaction) types.Currency {
	var burntAmount types.Currency
	for _, sfo := range tx.SiafundOutputs {
		if sfo.UnlockHash == types.BurnAddressUnlockHash {
			burntAmount = burntAmount.Add(sfo.Value)
		}
	}
	return burntAmount
}

// IsSpfTransportTx returns true if given tx is a valid SPF transport to Solana.
func IsSpfTransportTx(tx *types.Transaction) bool {
	// Ensure tx burns some SPFs.
	burntAmount := SpfBurntAmount(tx)
	if burntAmount.IsZero() {
		return false
	}
	// Ensure WholeTransaction flag is set in all signatures.
	for _, sig := range tx.TransactionSignatures {
		if !sig.CoveredFields.WholeTransaction {
			return false
		}
	}
	// Ensure there is valid Solana address in arbitrary data.
	for _, ad := range tx.ArbitraryData {
		if _, err := common.ExtractSolanaAddress(ad); err == nil {
			return true
		}
	}
	return false
}

func spfxEmissionTime(c types.Currency) time.Duration {
	minutes := common.DivCurrencyRoundUp(c, common.SpfPerMinute).Big().Int64()
	return time.Minute * time.Duration(minutes)
}

func (w *Wallet) spfxRegularUnlockHash(uh types.UnlockHash) bool {
	if _, ok := w.spfxPreminedAddrs[uh]; ok {
		return false
	}
	/*if uh == types.SpfxAirdropUnlockHash {
		return false
	}*/
	return true
}

func statusFromTransporter(s common.TransportStatus) (res types.SpfTransportStatus, err error) {
	switch s {
	case common.Unconfirmed:
		res = types.SubmittedToTransporter
	case common.InTheQueue:
		res = types.InTheQueue
	case common.SolanaTxCreated:
		res = types.InTheQueue
	case common.Completed:
		res = types.Completed
	default:
		err = fmt.Errorf("status %v is not convertable", s)
	}
	return
}

func equalStatus(remote common.TransportStatus, local types.SpfTransportStatus) bool {
	switch remote {
	case common.Unconfirmed:
		return local == types.SubmittedToTransporter
	case common.InTheQueue:
		return local == types.InTheQueue
	case common.SolanaTxCreated:
		return local == types.InTheQueue
	case common.Completed:
		return local == types.Completed
	}
	return false
}

func (w *Wallet) threadedMonitorSpfTransports() {
	if err := w.tg.Add(); err != nil {
		return
	}
	defer w.tg.Done()

	const monitorInterval = 10 * time.Minute
	for {
		select {
		case <-time.After(monitorInterval):
		case <-w.tg.StopChan():
			return
		}
		w.mu.Lock()
		allTransports, err := dbGetAllSpfTransports(w.dbTx)
		w.mu.Unlock()
		if err != nil {
			w.log.Println("Failed to load SPF transport records from database:", err)
			continue
		}
		recordsToUpdate := make(map[types.TransactionID]types.SpfTransportRecord)
		ctx := context.Background()
		for _, t := range allTransports {
			if t.Status == types.Completed {
				// Skip completed.
				continue
			}
			const minRecordAge = time.Hour
			if time.Since(t.Created) < minRecordAge {
				// Do not touch recently created records here, they might still be
				// updated by the actual Send function.
				continue
			}
			var newStatus types.SpfTransportStatus
			needToUpdateStatus := false
			// Check status on transporter.
			statusResp, err := w.transporterClient.TransportStatus(ctx, &transporter.TransportStatusRequest{
				BurnID: t.BurnID,
			})
			if err != nil {
				w.log.Printf("Failed to get status of the queue record %s from transporter: %v", t.BurnID.String(), err)
				continue
			}
			if statusResp.Status == common.NotFound {
				// Record is unknown to transporter, check if tx is confirmed.
				if t.Status != types.BurnCreated && t.Status != types.BurnBroadcasted {
					w.log.Printf("SPF transport record %s is not found on transporter, local status %s", t.BurnID.String(), t.Status.String())
					continue
				}
				confirmed, err := w.tpool.TransactionConfirmed(t.BurnID)
				if err != nil {
					w.log.Println("Failed to check if transaction is confirmed in tpool:", err)
					continue
				}
				if confirmed {
					// Burn tx was confirmed, try submitting to transporter.
					w.mu.Lock()
					burnTx, err := dbGetSpfBurn(w.dbTx, t.BurnID)
					w.mu.Unlock()
					if err != nil {
						w.log.Println("Failed to fetch burn txn set from database:", err)
						continue
					}
					if _, err := w.transporterClient.SubmitScpTx(
						ctx,
						&transporter.SubmitScpTxRequest{Transaction: burnTx},
					); err != nil {
						w.log.Println("Failed to submit confirmed tx to transporter (threadedMonitorSpfTransports):", err)
						continue
					}
					newStatus = types.SubmittedToTransporter
					needToUpdateStatus = true
				}
			} else if !equalStatus(statusResp.Status, t.Status) {
				newStatus, err = statusFromTransporter(statusResp.Status)
				if err != nil {
					w.log.Println("Failed to convert status from transporter:", err)
					continue
				}
				needToUpdateStatus = true
			}
			if needToUpdateStatus {
				newRecord := t.SpfTransportRecord
				newRecord.Status = newStatus
				recordsToUpdate[t.BurnID] = newRecord
			}
		}
		if err := w.updateSpfTransports(recordsToUpdate); err != nil {
			w.log.Println("Failed to save updated SPF transport records:", err)
		}
	}
}

func (w *Wallet) updateSpfTransports(records map[types.TransactionID]types.SpfTransportRecord) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for burnID, r := range records {
		if _, err := dbGetSpfTransport(w.dbTx, burnID); err == errNoKey {
			// Do not create new records here.
			continue
		}
		if err := dbPutSpfTransport(w.dbTx, types.SpfTransport{BurnID: burnID, SpfTransportRecord: r}); err != nil {
			return err
		}
	}
	return w.syncDB()
}

func (w *Wallet) putSpfBurn(burnID types.TransactionID, tx types.Transaction) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := dbPutSpfBurn(w.dbTx, burnID, tx); err != nil {
		return err
	}
	return w.syncDB()
}

func (w *Wallet) putSpfTransport(r types.SpfTransport) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := dbPutSpfTransport(w.dbTx, r); err != nil {
		return err
	}
	return w.syncDB()
}
