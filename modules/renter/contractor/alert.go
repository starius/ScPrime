package contractor

import "gitlab.com/SiaPrime/SiaPrime/modules"

// Alerts implements the modules.Alerter interface for the contractor. It returns
// all alerts of the contractor.
func (c *Contractor) Alerts() []modules.Alert {
	return c.staticAlerter.Alerts()
}
