package handler

import (
	"smart-home/event"
	"smart-home/internal"
	"time"
)

// HandlePower handles POWER<n> states. POWER / POWER1 is the device's "on" (for non-toggle
// devices it only means "seen online"); every relay's state is kept in Relays, and the output
// after the relays is the light. tele/STATE (Tele) never brings a device online, so it's
// ignored while offline.
func (h *EventHandler) HandlePower(state *internal.DeviceState, e *event.Power) {
	if e.Tele && !state.Online {
		return
	}
	now := time.Now()
	state.LastUpdate = &now
	lightIndex := state.LightPowerIndex()
	for index, on := range e.States {
		if index == 1 {
			state.On = new(on)
		}
		if index == lightIndex {
			state.Config.LightPower = new(on)
		}
		state.SetRelay(index, on)
	}
}
