package handler

import (
	"smart-home/event"
	"smart-home/internal"
	"time"
)

func (h *EventHandler) HandleState(state *internal.DeviceState, e *event.Status10) {
	now := time.Now()
	state.Current = &e.StatusSNS.Energy.Current
	state.Power = &e.StatusSNS.Energy.Power
	state.Voltage = &e.StatusSNS.Energy.Voltage
	state.Today = &e.StatusSNS.Energy.Today
	state.LastUpdate = &now
}
