package model

import (
	"fmt"
	"slices"
)

// MetricKeys are the sensor values the API reports (the JSON keys in device state and sensor events).
var MetricKeys = []string{
	"power", "current", "voltage",
	"temperature", "humidity", "co2",
	"pressure",
	"dcPower", "dcCurrent", "dcVoltage",
}

const (
	DashboardChartAuto = ""     // the first metric the device reports (frontend order)
	DashboardChartNone = "none" // no chart
)

// DashboardView is what the dashboard shows for a device. The zero value is "auto".
type DashboardView struct {
	// Chart is the sparkline metric: a MetricKeys entry, DashboardChartAuto or DashboardChartNone.
	Chart string `json:"chart"`
	// Values are the metrics shown as numbers, in display order; nil = every reported one.
	Values []string `json:"values"`
}

func ValidateDashboardView(v DashboardView) error {
	if v.Chart != DashboardChartAuto && v.Chart != DashboardChartNone && !slices.Contains(MetricKeys, v.Chart) {
		return fmt.Errorf("unknown chart metric %q", v.Chart)
	}
	for i, key := range v.Values {
		if !slices.Contains(MetricKeys, key) {
			return fmt.Errorf("unknown metric %q", key)
		}
		if slices.Contains(v.Values[:i], key) {
			return fmt.Errorf("duplicate metric %q", key)
		}
	}
	return nil
}
