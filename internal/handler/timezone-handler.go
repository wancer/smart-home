package handler

import (
	"smart-home/event"
	"smart-home/internal"
)

func (h *EventHandler) HandleTimezone(state *internal.DeviceState, e *event.Timezone) {
	state.Config.Timezone = new(string(*e))
}
