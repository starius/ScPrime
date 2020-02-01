package consensus

import "gitlab.com/SiaPrime/SiaPrime/modules"

// Alerts implements the Alerter interface for the consensusset.
func (c *ConsensusSet) Alerts() []modules.Alert {
	return []modules.Alert{}
}
