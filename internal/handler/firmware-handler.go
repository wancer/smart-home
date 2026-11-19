package handler

import (
	"smart-home/event"
	"smart-home/internal"
	"time"
)

// HandleFirmware stores the chip and firmware version (STATUS2) in the device profile.
func (h *EventHandler) HandleFirmware(state *internal.DeviceState, e *event.Status2) {
	d := state.Device
	builtAt := time.Time(e.StatusFWR.BuildDateTime)
	if d.Chip == e.StatusFWR.Hardware && d.FirmwareVersion == e.StatusFWR.Version &&
		d.FirmwareBuiltAt != nil && d.FirmwareBuiltAt.Equal(builtAt) {
		return
	}
	d.Chip = e.StatusFWR.Hardware
	d.FirmwareVersion = e.StatusFWR.Version
	d.FirmwareBuiltAt = &builtAt
	h.profiles.Save(d)
}
