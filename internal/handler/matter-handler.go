package handler

import (
	"log/slog"
	"smart-home/event"
	"smart-home/internal"
)

// HandleMatterInfo stores what a Matter bridge reports about a sensor (tele/<sensor>/INFO) in its
// profile: vendor/product/hardware version/serial as the chip, the software version as the
// firmware. Empty values keep what is stored.
func (h *EventHandler) HandleMatterInfo(state *internal.DeviceState, e *event.MatterInfo) {
	d := state.Device
	changed := false
	if hw := e.Hardware(); hw != "" && hw != d.Chip {
		d.Chip = hw
		changed = true
	}
	if e.SoftwareVersion != "" && e.SoftwareVersion != d.FirmwareVersion {
		d.FirmwareVersion = e.SoftwareVersion
		changed = true
	}
	if changed {
		h.profiles.Save(d)
	}
}

// HandleCommandResult passes a Matter bridge's reply (stat/<sensor>/RESULT) to the command waiting
// for it.
func (h *EventHandler) HandleCommandResult(state *internal.DeviceState, e *event.CommandResult) {
	if !state.CommandResult(*e) {
		slog.Info("[device][command] result without a waiting command", "deviceId", state.Device.ID,
			"command", e.Command, "result", e.Result)
	}
}
