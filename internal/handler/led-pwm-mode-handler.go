package handler

import (
	"smart-home/event"
	"smart-home/internal"
)

func (h *EventHandler) HandleLedPwmMode(state *internal.DeviceState, e *event.LedPwmMode) {
	state.Config.LedPwmMode = new(bool(*e))
}
