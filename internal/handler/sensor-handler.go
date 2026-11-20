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
		if !state.CanToggle() {
			// No relay: Tasmota answers POWER with "Command Unknown", so "on" mirrors availability.
			// With a relay the POWER reply (PublishStates below) sets it.
			state.On = new(true)
		}
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

	readings := e.Readings()
	r.Period, r.Power, r.ApparentPower, r.ReactivePower = readings.Period, readings.Power, readings.ApparentPower, readings.ReactivePower
	r.Voltage, r.Current = readings.Voltage, readings.Current
	r.CO2, r.Temperature, r.Humidity, r.DewPoint, r.Pressure = readings.CO2, readings.Temperature, readings.Humidity, readings.DewPoint, readings.Pressure
	r.DcVoltage, r.DcCurrent, r.DcPower = readings.DcVoltage, readings.DcCurrent, readings.DcPower

	return r
}
