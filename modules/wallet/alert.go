package wallet

import "gitlab.com/SiaPrime/SiaPrime/modules"

// Alerts implements the Alerter interface for the wallet.
func (w *Wallet) Alerts() []modules.Alert {
	return []modules.Alert{}
}
