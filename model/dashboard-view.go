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

// Legacy dashboard view and metric keys: the fixed per-metric JSON keys of the old API (device
// state, sensor events, and the dashboard view before series), still served to the Android app
// until it moves to series (then remove all of this).

const (
	LegacyChartAuto = ""     // the first metric the device reports
	LegacyChartNone = "none" // no chart
)

// LegacyDashboardView is the dashboard view by metric key (MetricKeyKinds).
type LegacyDashboardView struct {
	Chart  string   `json:"chart"`
	Values []string `json:"values"`
}

// MetricKeyKinds maps the old API's metric keys to series kinds.
var MetricKeyKinds = map[string]string{
	"power":       KindPower,
	"current":     KindCurrent,
	"voltage":     KindVoltage,
	"temperature": KindTemperature,
	"humidity":    KindHumidity,
	"co2":         KindCo2,
	"pressure":    KindPressure,
	"gasRatio":    KindGasRatio,
	"illuminance": KindIlluminance,
	"colorTemp":   KindColorTemp,
	"dcPower":     KindDcPower,
	"dcCurrent":   KindDcCurrent,
	"dcVoltage":   KindDcVoltage,
}

// KindMetricKey is the old API's metric key of a kind, "" if it has none.
func KindMetricKey(kind string) string {
	for key, k := range MetricKeyKinds {
		if k == kind {
			return key
		}
	}
	return ""
}

// ToLegacy maps a series view to metric keys by the series' kinds (kindOf returns "" for an
// unknown series). Series without a metric key are left out; a chart series without one is auto.
func (v DashboardView) ToLegacy(kindOf func(seriesId uint) string) LegacyDashboardView {
	var legacy LegacyDashboardView
	switch {
	case v.NoChart:
		legacy.Chart = LegacyChartNone
	case v.Chart != nil:
		legacy.Chart = KindMetricKey(kindOf(*v.Chart))
	}
	if v.Values != nil {
		legacy.Values = []string{}
		for _, id := range v.Values {
			if key := KindMetricKey(kindOf(id)); key != "" && !slices.Contains(legacy.Values, key) {
				legacy.Values = append(legacy.Values, key)
			}
		}
	}
	return legacy
}

// FromLegacy maps a metric-key view to series: each key takes the device's first preferred
// series of its kind (preferred: kind -> series IDs, best first). Keys the device has no
// series for are dropped (a chart key without one becomes auto). Unknown keys are an error.
func (v LegacyDashboardView) FromLegacy(preferred map[string][]uint) (DashboardView, error) {
	var view DashboardView
	pick := func(key string) (uint, bool, error) {
		kind, ok := MetricKeyKinds[key]
		if !ok {
			return 0, false, fmt.Errorf("unknown metric %q", key)
		}
		if ids := preferred[kind]; len(ids) > 0 {
			return ids[0], true, nil
		}
		return 0, false, nil
	}
	switch v.Chart {
	case LegacyChartAuto:
	case LegacyChartNone:
		view.NoChart = true
	default:
		id, ok, err := pick(v.Chart)
		if err != nil {
			return view, err
		}
		if ok {
			view.Chart = &id
		}
	}
	if v.Values != nil {
		view.Values = []uint{}
		for _, key := range v.Values {
			id, ok, err := pick(key)
			if err != nil {
				return view, err
			}
			if ok && !slices.Contains(view.Values, id) {
				view.Values = append(view.Values, id)
			}
		}
	}
	return view, nil
}
