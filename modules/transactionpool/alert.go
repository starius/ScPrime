package transactionpool

import "gitlab.com/SiaPrime/SiaPrime/modules"

// Alerts implements the modules.Alerter interface for the transactionpool.
func (tpool *TransactionPool) Alerts() []modules.Alert {
	return []modules.Alert{}
}
