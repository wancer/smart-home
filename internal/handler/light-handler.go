package handler

import (
	"smart-home/event"
	"smart-home/internal"
)

func (h *EventHandler) HandleTemplate(state *internal.DeviceState, e *event.Template) {
	hw := internal.DetectLedHardware(e.Gpio)
	state.Config.LedHardware = &hw
}

func (h *EventHandler) HandleLight(state *internal.DeviceState, e *event.Light) {
	if e.Power != nil && e.PowerIndex == state.LightPowerIndex() {
		state.Config.LightPower = new(*e.Power)
	}
	if e.Dimmer != nil {
		state.Config.Dimmer = new(*e.Dimmer)
	}
	if e.Hue != nil && e.Saturation != nil {
		state.Config.Hue = new(*e.Hue)
		state.Config.Saturation = new(*e.Saturation)
	}
}
