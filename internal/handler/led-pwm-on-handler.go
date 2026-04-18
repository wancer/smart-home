package handler

import (
	"smart-home/event"
	"smart-home/internal"
)

func (h *EventHandler) HandleLedPwmOn(state *internal.DeviceState, e *event.LedPwmOn) {
	state.Config.LedPwmOn = new(uint(*e))
}
