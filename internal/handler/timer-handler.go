package handler

import (
	"smart-home/event"
	"smart-home/internal"
)

func (h *EventHandler) HandleTimer(state *internal.DeviceState, e *event.Timer) {
	if state == nil || e.N < 1 || e.N > 16 {
		return
	}
	state.Config.Timers[e.N-1] = &internal.TimerConfig{
		Enable: e.Enable,
		Mode:   e.Mode,
		Time:   e.Time,
		Window: e.Window,
		Days:   e.Days,
		Repeat: e.Repeat,
		Output: e.Output,
		Action: e.Action,
	}
}
