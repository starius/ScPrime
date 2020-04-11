package host

import (
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
)

func (h *Host) calculatePriceByResource(resourceType types.Specifier, resourceAmount int64) types.Currency {
	h.mu.RLock()
	settings := h.externalSettings()
	h.mu.RUnlock()
	var resourceCost types.Currency
	switch resourceType {
	case modules.DownloadBytes:
		resourceCost = settings.DownloadBandwidthPrice.Mul64(uint64(resourceAmount))
	case modules.UploadBytes:
		resourceCost = settings.UploadBandwidthPrice.Mul64(uint64(resourceAmount))
	case modules.SectorAccesses:
		resourceCost = settings.SectorAccessPrice.Mul64(uint64(resourceAmount))
	case modules.KeyValueSets:
		resourceCost = settings.KeyValueSetPrice.Mul64(uint64(resourceAmount))
	case modules.KeyValueGets:
		resourceCost = settings.KeyValueGetPrice.Mul64(uint64(resourceAmount))
	case modules.KeyValueDeletes:
		resourceCost = settings.KeyValueDeletePrice.Mul64(uint64(resourceAmount))
	}
	totalCost := settings.BaseRPCPrice.Add(resourceCost)
	return totalCost
}
