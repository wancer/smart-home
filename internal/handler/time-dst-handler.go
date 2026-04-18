package handler

import (
	"smart-home/event"
	"smart-home/internal"
)

func (h *EventHandler) HandleTimeDst(state *internal.DeviceState, e *event.TimeDst) {
	state.Config.TimeDst = &internal.TimeSwitch{
		Day:        e.Day,
		Hemisphere: e.Hemisphere,
		Hour:       e.Hour,
		Month:      e.Month,
		Offset:     e.Offset,
		Week:       e.Week,
	}
}
