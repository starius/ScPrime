package explorer

import "gitlab.com/scpcorp/ScPrime/modules"

// Alerts implements the modules.Alerter interface for the explorer.
func (e *Explorer) Alerts() []modules.Alert {
	return []modules.Alert{}
}
