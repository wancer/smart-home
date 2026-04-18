package handler

import (
	"smart-home/event"
	"smart-home/internal"
)

func (c *EventHandler) HandleLedState(state *internal.DeviceState, e *event.LedState) {
	state.Config.LedState = new(uint(*e))
}
