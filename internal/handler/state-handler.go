package handler

import (
	"smart-home/event"
	"smart-home/internal"
	"smart-home/model"
	"time"
)

func (h *EventHandler) HandleState(state *internal.DeviceState, e *event.Status10) {
	now := time.Now()
	state.ChipTemperature = e.StatusSNS.ChipTemperature()

	if state.Device.SensorType == model.SensorTypeIR {
		return // no sensors
	}
	readings := e.StatusSNS.Readings()
	state.SetReadings(&readings)
	if e.StatusSNS.Energy != nil {
		state.LastUpdate = &now
	}
}
