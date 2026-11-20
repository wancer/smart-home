package handler

import (
	"smart-home/event"
	"smart-home/internal"
	"time"
)

// HandleState handles the STATUS10 reply: current sensor values, and sensor type detection
// (forced while a manual profile refresh is pending).
func (h *EventHandler) HandleState(state *internal.DeviceState, e *event.Status10) {
	if e.StatusSNS == nil {
		return
	}
	now := time.Now()
	sns := e.StatusSNS
	state.ChipTemperature = sns.ChipTemperature()

	force := state.ProfileRefresh
	state.ProfileRefresh = false
	if state.ApplyDetection(sns, force) {
		h.profiles.Save(state.Device)
	}

	if !internal.HasSensorData(state.Device.SensorType, sns) {
		return
	}
	readings := sns.Readings()
	state.SetReadings(&readings)
	if sns.Energy != nil {
		state.LastUpdate = &now
	}
}
