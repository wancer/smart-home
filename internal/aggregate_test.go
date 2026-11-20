package internal

import (
	"smart-home/model"
	"testing"
	"time"
)

func TestGroupReadingsNewSensors(t *testing.T) {
	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	reading := func(offset time.Duration, pressure, volts, amps, watts float32) *model.SensorEvent {
		return &model.SensorEvent{
			DeviceId: 1, RealTime: at.Add(offset),
			Pressure: new(pressure), DcVoltage: new(volts), DcCurrent: new(amps), DcPower: new(watts),
		}
	}
	buckets := groupReadings([]*model.SensorEvent{
		reading(0, 1000, 5.0, 0.2, 1.0),
		reading(time.Minute, 1010, 5.2, 0.4, 2.0),
	}, 300)

	agg := buckets[at]
	if agg == nil || agg.Count != 2 {
		t.Fatalf("expected one bucket of 2, got %+v", buckets)
	}
	near := func(name string, got *float32, want float32) {
		t.Helper()
		if got == nil || *got-want > 1e-4 || want-*got > 1e-4 {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	near("PressureAvg", agg.PressureAvg, 1005)
	near("PressureMin", agg.PressureMin, 1000)
	near("PressureMax", agg.PressureMax, 1010)
	near("DcVoltageAvg", agg.DcVoltageAvg, 5.1)
	near("DcCurrentMax", agg.DcCurrentMax, 0.4)
	near("DcPowerMin", agg.DcPowerMin, 1.0)
	if agg.PowerAvg != nil || agg.TemperatureAvg != nil {
		t.Error("unreported values must stay nil")
	}
}
