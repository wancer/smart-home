package handler

import (
	"smart-home/event"
	"smart-home/internal"
)

func (h *EventHandler) HandleTelePeriod(state *internal.DeviceState, e *event.TelePeriod) {
	state.Config.TelePeriod = new(uint(*e))
}
