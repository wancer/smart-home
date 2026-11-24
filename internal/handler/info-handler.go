package handler

import (
	"smart-home/event"
	"smart-home/internal"
)

// HandleInfo takes the firmware version (INFO1) and restart reason (INFO3) that ESPHome devices
// publish on connect, in place of the STATUS2 / STATUS1 replies Tasmota devices are polled for.
func (h *EventHandler) HandleInfo(state *internal.DeviceState, e *event.Info) {
	if e.Info1 != nil && e.Info1.Version != "" && e.Info1.Version != state.Device.FirmwareVersion {
		state.Device.FirmwareVersion = e.Info1.Version
		h.profiles.Save(state.Device)
	}
	if e.Info3 != nil && e.Info3.RestartReason != "" {
		state.RestartReason = new(e.Info3.RestartReason)
		h.dispatcher.Dispatch(&event.DeviceRestartInfo{DeviceId: state.Device.ID})
	}
}
