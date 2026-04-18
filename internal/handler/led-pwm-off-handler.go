package handler

import (
	"smart-home/event"
	"smart-home/internal"
)

func (h *EventHandler) HandleLedPwmOff(state *internal.DeviceState, e *event.LedPwmOff) {
	state.Config.LedPwmOff = new(uint(*e))
}
