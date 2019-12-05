package miner

import "gitlab.com/SiaPrime/SiaPrime/modules"

// Alerts implements the modules.Alerter interface for the miner.
func (m *Miner) Alerts() []modules.Alert {
	return []modules.Alert{}
}
