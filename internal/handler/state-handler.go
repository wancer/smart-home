package handler

import (
	"log/slog"
	"smart-home/event"
	"smart-home/internal"
	"smart-home/model"
	"time"
)

func (h *EventHandler) HandleState(state *internal.DeviceState, e *event.Status10) {
	now := time.Now()

	switch state.Device.SensorType {
	case model.SensorTypeEnergy:
		state.Current = &e.StatusSNS.Energy.Current
		state.Power = &e.StatusSNS.Energy.Power
		state.Voltage = &e.StatusSNS.Energy.Voltage
		state.Today = &e.StatusSNS.Energy.Today
		state.LastUpdate = &now
	case model.SensorTypeCo2:
		state.CO2E = &e.StatusSNS.Co2.CO2E
		state.CO2 = &e.StatusSNS.Co2.CO2
		state.Temperature = &e.StatusSNS.Co2.Temperature
		state.Humidity = &e.StatusSNS.Co2.Humidity
		state.DewPoint = &e.StatusSNS.Co2.DewPoint
	case model.SensorTypeTempHumid:
		state.Temperature = &e.StatusSNS.TempHum.Temperature
		state.Humidity = &e.StatusSNS.TempHum.Humidity
		state.DewPoint = &e.StatusSNS.TempHum.DewPoint
	default:
		slog.Error("UNKOWN_TYPE: " + state.Device.SensorType)
	}
}
