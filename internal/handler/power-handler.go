package handler

import (
	"smart-home/event"
	"smart-home/internal"
	"time"
)

// HandlePower handles POWER / POWER1. For non-toggle devices "on" only means "seen online";
// if the device has no relay, POWER1 is its light.
func (h *EventHandler) HandlePower(state *internal.DeviceState, e *event.Power) {
	now := time.Now()
	state.On = new(bool(*e))
	state.LastUpdate = &now
	if state.LightPowerIndex() == 1 {
		state.Config.LightPower = new(bool(*e))
	}
}
