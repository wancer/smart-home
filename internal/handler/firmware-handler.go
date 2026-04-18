package handler

import (
	"smart-home/event"
	"smart-home/internal"
	"time"
)

func (h *EventHandler) HandleFirmware(state *internal.DeviceState, e *event.Status2) {
	state.Firmware.Version = &e.StatusFWR.Version
	converted := time.Time(e.StatusFWR.BuildDateTime)
	state.Firmware.BuiltAt = &converted
}
