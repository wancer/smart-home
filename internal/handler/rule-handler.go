package handler

import (
	"smart-home/event"
	"smart-home/internal"
)

func (h *EventHandler) HandleRule(state *internal.DeviceState, e *event.Rule) {
	if state == nil || e.N < 1 || e.N > 3 {
		return
	}
	state.Config.Rules[e.N-1] = &internal.RuleConfig{
		State: e.State,
		Once:  e.Once,
		Rules: e.Rules,
		Free:  e.Free,
	}
}
