package handler

import (
	"fmt"
	"log/slog"
	"smart-home/event"
	"smart-home/internal"
	"smart-home/model"
	"time"
)

func (h *EventHandler) HandleSensorEvent(state *internal.DeviceState, e *event.SensorEvent) {
	now := time.Now()

	m := toModel(e, state.Device, &now)
	h.storage.Store(m)

	if !state.Online {
		slog.Info(fmt.Sprintf("Getting %s online", state.Device.Name))
		state.Online = true
		h.p.PublishStates(state.Device)
	}

	switch state.Device.SensorType {
	case model.SensorTypeEnergy:
		h.storage.StoreDaily(e, state.Device.ID)

		state.Current = &e.Energy.Current
		state.Power = &e.Energy.Power
		state.Voltage = &e.Energy.Voltage
		state.Today = &e.Energy.Today
		state.LastUpdate = &now
	case model.SensorTypeCo2:
		state.CO2E = &e.Co2.CO2E
		state.CO2 = &e.Co2.CO2
		state.Temperature = &e.Co2.Temperature
		state.Humidity = &e.Co2.Humidity
		state.DewPoint = &e.Co2.DewPoint
	case model.SensorTypeTempHumid:
		state.Temperature = &e.TempHum.Temperature
		state.Humidity = &e.TempHum.Humidity
		state.DewPoint = &e.TempHum.DewPoint
	default:
		slog.Error("UNKOWN_TYPE: " + state.Device.SensorType)
	}
}

func toModel(e *event.SensorEvent, device *model.Device, now *time.Time) *model.SensorEvent {
	r := &model.SensorEvent{}
	r.DeviceId = device.ID
	r.RealTime = *now
	r.DeviceTime = time.Time(e.Time)

	switch device.SensorType {
	case model.SensorTypeEnergy:
		r.Period = &e.Energy.Period
		r.Power = &e.Energy.Power
		r.ApparentPower = &e.Energy.ApparentPower
		r.ReactivePower = &e.Energy.ReactivePower
		r.Voltage = &e.Energy.Voltage
		r.Current = &e.Energy.Current
	case model.SensorTypeCo2:
		r.CO2e = &e.Co2.CO2E
		r.CO2 = &e.Co2.CO2
		r.Temperature = &e.Co2.Temperature
		r.Humidity = &e.Co2.Humidity
		r.DewPoint = &e.Co2.DewPoint
	case model.SensorTypeTempHumid:
		r.Temperature = &e.TempHum.Temperature
		r.Humidity = &e.TempHum.Humidity
		r.DewPoint = &e.TempHum.DewPoint
	default:
		slog.Error("UNKOWN_TYPE: " + device.SensorType)
	}

	return r
}
