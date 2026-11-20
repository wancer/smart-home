package handler

import (
	"smart-home/event"
	"smart-home/internal"
)

// HandleStatus1 keeps the reason of the device's last restart (polled when it comes online).
func (h *EventHandler) HandleStatus1(state *internal.DeviceState, e *event.Status1) {
	if e.StatusPRM == nil {
		return
	}
	state.RestartReason = new(e.StatusPRM.RestartReason)
	h.dispatcher.Dispatch(&event.DeviceRestartInfo{DeviceId: state.Device.ID})
}
