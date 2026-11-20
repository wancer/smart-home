package handler

import (
	"smart-home/event"
	"smart-home/internal"
	"time"
)

// HandleFirmware stores the chip, firmware version, core and SDK (STATUS2) in the device profile.
func (h *EventHandler) HandleFirmware(state *internal.DeviceState, e *event.Status2) {
	d := state.Device
	builtAt := time.Time(e.StatusFWR.BuildDateTime)
	if d.Chip == e.StatusFWR.Hardware && d.FirmwareVersion == e.StatusFWR.Version &&
		d.FirmwareCore == e.StatusFWR.Core && d.FirmwareSdk == e.StatusFWR.SDK &&
		d.FirmwareBuiltAt != nil && d.FirmwareBuiltAt.Equal(builtAt) {
		return
	}
	d.Chip = e.StatusFWR.Hardware
	d.FirmwareVersion = e.StatusFWR.Version
	d.FirmwareBuiltAt = &builtAt
	d.FirmwareCore = e.StatusFWR.Core
	d.FirmwareSdk = e.StatusFWR.SDK
	h.profiles.Save(d)
}
