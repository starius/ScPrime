package host

import "gitlab.com/scpcorp/ScPrime/modules"

// Alerts implements the modules.Alerter interface for the host.
func (h *Host) Alerts() (crit, err, warn []modules.Alert) {
	crit, err, warn = h.staticAlerter.Alerts()

	h.mu.RLock()
	storageManager := h.StorageManager
	h.mu.RUnlock()
	if storageManager != nil {
		smCrit, smErr, smWarn := storageManager.Alerts()
		crit = append(crit, smCrit...)
		err = append(err, smErr...)
		warn = append(warn, smWarn...)
	}

	return crit, err, warn
}

// tryUnregisterInsufficientCollateralBudgetAlert will be called when the host
// updates his collateral budget setting or when the locked storage collateral
// gets updated (in a way the updated storage collateral is lower).
func (h *Host) tryUnregisterInsufficientCollateralBudgetAlert() {
	// Unregister the alert if the collateral budget is enough to support cover
	// a contract's max collateral and the currently locked storage collateral
	if h.financialMetrics.LockedStorageCollateral.Add(h.settings.MaxCollateral).Cmp(h.settings.CollateralBudget) <= 0 {
		h.staticAlerter.UnregisterAlert(modules.AlertIDHostInsufficientCollateral)
	}
}
