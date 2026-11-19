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
	state.ChipTemperature = e.ChipTemperature()

	if state.ApplyDetection(e, false) {
		h.profiles.Save(state.Device)
	}

	// IR remotes, unrecognised devices, or a message without the device's sensor
	if !internal.HasSensorData(state.Device.SensorType, e) {
		h.handleBareTelemetry(state, e, &now)
		return
	}

	m := toModel(e, state.Device, &now)
	h.storage.Store(m)

	state.LastUpdate = &now
	state.LastTelemetry = &now
	if !state.Online {
		slog.Info(fmt.Sprintf("Getting %s online", state.Device.Name))
		state.Online = true
		h.p.PublishStates(state.Device)
		h.dispatcher.Dispatch(&event.DeviceAvailability{DeviceId: state.Device.ID, Online: true})
	}

	switch state.Device.SensorType {
	case model.SensorTypeEnergy:
		h.storage.StoreDaily(e, state.Device.ID)

		state.Current = &e.Energy.Current
		state.Power = &e.Energy.Power
		state.Voltage = &e.Energy.Voltage
		state.Today = &e.Energy.Today
	case model.SensorTypeCo2:
		state.CO2 = &e.Co2.CO2
		state.Temperature = &e.Co2.Temperature
		state.Humidity = &e.Co2.Humidity
		state.DewPoint = &e.Co2.DewPoint
	case model.SensorTypeTempHumid:
		state.Temperature = &e.TempHum.Temperature
		state.Humidity = &e.TempHum.Humidity
		state.DewPoint = &e.TempHum.DewPoint
	}

	h.dispatcher.Dispatch(&event.SensorsUpdated{DeviceId: state.Device.ID})
}

// handleBareTelemetry treats telemetry without the device's sensor data (IR remotes and
// unrecognised devices: only the ESP32 chip temperature, SetOption146 1) as a sign of life,
// on top of LWT. A sensor device that sent a message without its sensor only gets the
// chip temperature.
func (h *EventHandler) handleBareTelemetry(state *internal.DeviceState, e *event.SensorEvent, now *time.Time) {
	if state.ChipTemperature != nil {
		h.storage.Store(&model.SensorEvent{
			DeviceId:        state.Device.ID,
			RealTime:        now.UTC(),
			DeviceTime:      time.Time(e.Time),
			ChipTemperature: state.ChipTemperature,
		})
	}
	if !state.Device.AvailabilityFromLwt() {
		return
	}

	state.LastUpdate = now
	state.LastTelemetry = now
	if !state.Online {
		slog.Info(fmt.Sprintf("Getting %s online (telemetry)", state.Device.Name))
		state.Online = true
		if !state.CanToggle() {
			state.On = new(true) // "on" mirrors availability; for relays the POWER reply sets it
		}
		h.p.PublishStates(state.Device)
		h.dispatcher.Dispatch(&event.DeviceAvailability{DeviceId: state.Device.ID, Online: true})
	}
}

func toModel(e *event.SensorEvent, device *model.Device, now *time.Time) *model.SensorEvent {
	r := &model.SensorEvent{}
	r.DeviceId = device.ID
	r.RealTime = now.UTC()
	r.DeviceTime = time.Time(e.Time)
	r.ChipTemperature = e.ChipTemperature()

	switch device.SensorType {
	case model.SensorTypeEnergy:
		r.Period = &e.Energy.Period
		r.Power = &e.Energy.Power
		r.ApparentPower = &e.Energy.ApparentPower
		r.ReactivePower = &e.Energy.ReactivePower
		r.Voltage = &e.Energy.Voltage
		r.Current = &e.Energy.Current
	case model.SensorTypeCo2:
		r.CO2 = &e.Co2.CO2
		r.Temperature = &e.Co2.Temperature
		r.Humidity = &e.Co2.Humidity
		r.DewPoint = &e.Co2.DewPoint
	case model.SensorTypeTempHumid:
		r.Temperature = &e.TempHum.Temperature
		r.Humidity = &e.TempHum.Humidity
		r.DewPoint = &e.TempHum.DewPoint
	}

	return r
}
