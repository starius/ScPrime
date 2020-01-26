package host

import "gitlab.com/SiaPrime/SiaPrime/modules"

// Alerts implements the modules.Alerter interface for the host.
func (h *Host) Alerts() []modules.Alert {
	return []modules.Alert{}
}
