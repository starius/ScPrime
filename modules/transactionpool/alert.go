package transactionpool

import "gitlab.com/scpcorp/ScPrime/modules"

// Alerts implements the modules.Alerter interface for the transactionpool.
func (tpool *TransactionPool) Alerts() []modules.Alert {
	return []modules.Alert{}
}
