package handler

import (
	"log/slog"
	"smart-home/event"
	"smart-home/internal"
	"smart-home/model"
	"time"
)

// HandleLwt tracks availability for IR remotes. If they also send telemetry (ESP32 chip
// temperature), HandleSensorEvent and StateMonitor use it as well. Other device types
// come online with sensor events, but go offline right away on LWT "Offline".
func (h *EventHandler) HandleLwt(state *internal.DeviceState, e *event.Lwt) {
	if state.Device.SensorType != model.SensorTypeIR {
		if !bool(*e) && state.Online {
			slog.Info("Marking as offline: LWT", "deviceId", state.Device.ID)
			h.dispatcher.Dispatch(&event.InternalOfflineEvent{DeviceId: state.Device.ID})
		}
		return
	}

	now := time.Now()
	changed := state.Online != bool(*e)
	state.Online = bool(*e)
	// IR has no power control: "on" mirrors availability, and nil means offline (as for other devices)
	state.On = nil
	if state.Online {
		state.On = new(true)
	}
	state.LastUpdate = &now
	if !state.Online {
		state.LastTelemetry = nil
		state.ChipTemperature = nil
		state.Uptime = nil
		state.LoadAvg = nil
	}

	if changed && state.Online {
		h.p.PublishStates(state.Device) // chip temperature + TelePeriod
	}
	if changed {
		h.dispatcher.Dispatch(&event.DeviceAvailability{DeviceId: state.Device.ID, Online: state.Online})
	}
}
