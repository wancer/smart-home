package handler

import (
	"smart-home/event"
	"smart-home/internal"
)

func (h *EventHandler) HandleLedPower(state *internal.DeviceState, e *event.LedPower) {
	state.Config.LedPower = new(bool(*e))
}
