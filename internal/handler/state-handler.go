package handler

import (
	"smart-home/event"
	"smart-home/internal"
	"smart-home/model"
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
	switch state.Device.SensorType {
	case model.SensorTypeEnergy:
		state.Current = &sns.Energy.Current
		state.Power = &sns.Energy.Power
		state.Voltage = &sns.Energy.Voltage
		state.Today = &sns.Energy.Today
		state.LastUpdate = &now
	case model.SensorTypeCo2:
		state.CO2 = &sns.Co2.CO2
		state.Temperature = &sns.Co2.Temperature
		state.Humidity = &sns.Co2.Humidity
		state.DewPoint = &sns.Co2.DewPoint
	case model.SensorTypeTempHumid:
		state.Temperature = &sns.TempHum.Temperature
		state.Humidity = &sns.TempHum.Humidity
		state.DewPoint = &sns.TempHum.DewPoint
	}
}
