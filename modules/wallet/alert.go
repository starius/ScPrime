package wallet

import "gitlab.com/scpcorp/ScPrime/modules"

// Alerts implements the Alerter interface for the wallet.
func (w *Wallet) Alerts() (crit, err, warn []modules.Alert) {
	return []modules.Alert{}, []modules.Alert{}, []modules.Alert{}
}
