package model

import (
	"fmt"
	"slices"
)

// DashboardView is what the dashboard shows for a device, by series (stored in device.dashboard
// as JSON). The zero value is "auto": the first reported series as sparkline, every visible
// reported series as values.
type DashboardView struct {
	// Chart is the sparkline series; nil = auto (or no chart when NoChart).
	Chart   *uint `json:"chart"`
	NoChart bool  `json:"noChart"`
	// Values are the series shown as numbers, in display order; nil = every visible reported one.
	Values []uint `json:"values"`
}

// IsAuto reports whether the view is fully automatic (stored as NULL).
func (v DashboardView) IsAuto() bool {
	return v.Chart == nil && !v.NoChart && v.Values == nil
}

// ValidateDashboardView checks that every series of the view belongs to the device (owned)
// and that values have no duplicates.
func ValidateDashboardView(v DashboardView, owned func(seriesId uint) bool) error {
	if v.Chart != nil && v.NoChart {
		return fmt.Errorf("chart and noChart are exclusive")
	}
	if v.Chart != nil && !owned(*v.Chart) {
		return fmt.Errorf("unknown chart series %d", *v.Chart)
	}
	for i, id := range v.Values {
		if !owned(id) {
			return fmt.Errorf("unknown series %d", id)
		}
		if slices.Contains(v.Values[:i], id) {
			return fmt.Errorf("duplicate series %d", id)
		}
	}
	return nil
}
