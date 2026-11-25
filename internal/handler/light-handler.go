package handler

import (
	"reflect"
	"smart-home/event"
	"smart-home/internal"
	"smart-home/model"
	"time"
)

// HandleTemplate stores the GPIO template in the device profile; an IR GPIO may set the type.
func (h *EventHandler) HandleTemplate(state *internal.DeviceState, e *event.Template) {
	d := state.Device
	tpl := &model.DeviceTemplate{Name: e.Name, Gpio: e.Gpio, Flag: e.Flag, Base: e.Base}
	changed := d.Template == nil || !reflect.DeepEqual(*d.Template, *tpl)
	d.Template = tpl
	if state.ApplyDetection(nil, false) {
		changed = true
	}
	if changed {
		h.profiles.Save(d)
	}
}

func (h *EventHandler) HandleLight(state *internal.DeviceState, e *event.Light) {
	if e.Power != nil && e.PowerIndex == state.LightPowerIndex() {
		state.Config.LightPower = new(*e.Power)
	}
	// POWER1 in a light reply is the device's on/off, as in HandlePower
	// (web_socket.go pushes it as a `state` event).
	if e.Power != nil && e.PowerIndex == 1 {
		now := time.Now()
		state.On = new(*e.Power)
		state.SetRelay(1, *e.Power)
		state.LastUpdate = &now
	}
	if e.Dimmer != nil {
		state.Config.Dimmer = new(*e.Dimmer)
	}
	if e.Hue != nil && e.Saturation != nil {
		state.Config.Hue = new(*e.Hue)
		state.Config.Saturation = new(*e.Saturation)
	}
}
