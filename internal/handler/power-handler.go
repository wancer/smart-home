package handler

import (
	"smart-home/event"
	"smart-home/internal"
	"time"
)

func (h *EventHandler) HandlePower(state *internal.DeviceState, e *event.Power) {
	now := time.Now()
	state.On = new(bool(*e))
	state.LastUpdate = &now
}
