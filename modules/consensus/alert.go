package consensus

import "gitlab.com/scpcorp/ScPrime/modules"

// Alerts implements the Alerter interface for the consensusset.
func (c *ConsensusSet) Alerts() []modules.Alert {
	return []modules.Alert{}
}
