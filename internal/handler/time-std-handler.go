package handler

import (
	"smart-home/event"
	"smart-home/internal"
)

func (h *EventHandler) HandleTimeStd(state *internal.DeviceState, e *event.TimeStd) {
	state.Config.TimeStd = &internal.TimeSwitch{
		Day:        e.Day,
		Hemisphere: e.Hemisphere,
		Hour:       e.Hour,
		Month:      e.Month,
		Offset:     e.Offset,
		Week:       e.Week,
	}
}
