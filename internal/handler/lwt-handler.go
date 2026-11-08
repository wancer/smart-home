package handler

import (
	"smart-home/event"
	"smart-home/internal"
	"smart-home/model"
	"time"
)

// HandleLwt tracks availability for devices that send no sensor telemetry (IR).
// Other device types derive their online state from sensor events.
func (h *EventHandler) HandleLwt(state *internal.DeviceState, e *event.Lwt) {
	if state.Device.SensorType != model.SensorTypeIR {
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

	if changed {
		h.dispatcher.Dispatch(&event.DeviceAvailability{DeviceId: state.Device.ID, Online: state.Online})
	}
}
