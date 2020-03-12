package gateway

import "gitlab.com/scpcorp/ScPrime/modules"

// Alerts implements the modules.Alerter interface for the gateway.
func (g *Gateway) Alerts() []modules.Alert {
	return g.staticAlerter.Alerts()
}
