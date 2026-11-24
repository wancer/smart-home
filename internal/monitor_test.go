package internal

import (
	"smart-home/model"
	"testing"
	"time"
)

func TestOfflineTimeout(t *testing.T) {
	at := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		platform string
		setting  uint
		tele     *uint
		sensorAt []time.Duration // tele/SENSOR messages while online, relative to at
		want     time.Duration
	}{
		{"tasmota default TelePeriod", model.PlatformTasmota, 0, nil, nil, 630 * time.Second},
		{"tasmota reported TelePeriod", model.PlatformTasmota, 0, new(uint(10)), nil, time.Minute},
		{"tasmota ignores the observed interval", model.PlatformTasmota, 0, nil, []time.Duration{0, 15 * time.Second}, 630 * time.Second},
		{"setting wins", model.PlatformEsphome, 90, nil, []time.Duration{0, 15 * time.Second}, 90 * time.Second},
		{"esphome before two messages", model.PlatformEsphome, 0, nil, []time.Duration{0}, 630 * time.Second},
		{"esphome observed interval", model.PlatformEsphome, 0, nil, []time.Duration{0, 5 * time.Minute}, 630 * time.Second},
		{"esphome short interval: at least a minute", model.PlatformEsphome, 0, nil, []time.Duration{0, 15 * time.Second}, time.Minute},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			state := &DeviceState{
				Online: true,
				Device: &model.Device{Platform: c.platform, OfflineTimeout: c.setting},
				Config: &DeviceConfig{TelePeriod: c.tele},
			}
			for _, d := range c.sensorAt {
				now := at.Add(d)
				state.MarkTelemetry(&now)
			}
			if got := OfflineTimeout(state); got != c.want {
				t.Errorf("OfflineTimeout = %v, want %v", got, c.want)
			}
		})
	}
}

// A tele/SENSOR right after coming online measures nothing: the gap since the last message
// before going offline isn't an interval.
func TestMarkTelemetryOnlyWhileOnline(t *testing.T) {
	state := &DeviceState{Device: &model.Device{Platform: model.PlatformEsphome}, Config: &DeviceConfig{}}
	first := time.Now()
	state.MarkTelemetry(&first)
	later := first.Add(time.Hour)
	state.MarkTelemetry(&later)
	if state.TelemetryInterval != 0 {
		t.Errorf("TelemetryInterval = %v while offline, want 0", state.TelemetryInterval)
	}
}
