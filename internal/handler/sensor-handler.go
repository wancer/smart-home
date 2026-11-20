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

	if state.Device.SensorType == model.SensorTypeIR {
		h.handleIrTelemetry(state, e, &now)
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

	if e.Energy != nil {
		h.storage.StoreDaily(e, state.Device.ID)
	}
	readings := e.Readings()
	state.SetReadings(&readings)

	h.dispatcher.Dispatch(&event.SensorsUpdated{DeviceId: state.Device.ID})
}

// handleIrTelemetry treats an IR remote's telemetry (it only carries the ESP32 chip
// temperature, SetOption146 1) as a sign of life, on top of LWT.
func (h *EventHandler) handleIrTelemetry(state *internal.DeviceState, e *event.SensorEvent, now *time.Time) {
	if state.Device.StoreChipTemp && state.ChipTemperature != nil {
		h.storage.Store(&model.SensorEvent{
			DeviceId:        state.Device.ID,
			RealTime:        now.UTC(),
			DeviceTime:      time.Time(e.Time),
			ChipTemperature: state.ChipTemperature,
		})
	}

	state.LastUpdate = now
	state.LastTelemetry = now
	if !state.Online {
		slog.Info(fmt.Sprintf("Getting %s online (telemetry)", state.Device.Name))
		state.Online = true
		state.On = new(true) // IR "on" mirrors availability
		h.p.PublishStates(state.Device)
		h.dispatcher.Dispatch(&event.DeviceAvailability{DeviceId: state.Device.ID, Online: true})
	}
}

func toModel(e *event.SensorEvent, device *model.Device, now *time.Time) *model.SensorEvent {
	r := &model.SensorEvent{}
	r.DeviceId = device.ID
	r.RealTime = now.UTC()
	r.DeviceTime = time.Time(e.Time)
	if device.StoreChipTemp {
		r.ChipTemperature = e.ChipTemperature()
	}

	readings := e.Readings()
	r.Period, r.Power, r.ApparentPower, r.ReactivePower = readings.Period, readings.Power, readings.ApparentPower, readings.ReactivePower
	r.Voltage, r.Current = readings.Voltage, readings.Current
	r.CO2, r.Temperature, r.Humidity, r.DewPoint, r.Pressure = readings.CO2, readings.Temperature, readings.Humidity, readings.DewPoint, readings.Pressure
	r.DcVoltage, r.DcCurrent, r.DcPower = readings.DcVoltage, readings.DcCurrent, readings.DcPower

	return r
}
