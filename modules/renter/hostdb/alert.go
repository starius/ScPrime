package hostdb

import "gitlab.com/SiaPrime/SiaPrime/modules"

// Alerts implements the modules.Alerter interface for the hostdb. It returns
// all alerts of the hostdb.
func (hdb *HostDB) Alerts() []modules.Alert {
	return hdb.staticAlerter.Alerts()
}
