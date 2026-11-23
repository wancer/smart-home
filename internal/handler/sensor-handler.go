package handler

import (
	"fmt"
	"log/slog"
	"smart-home/event"
	"smart-home/internal"
	"time"
)

func (h *EventHandler) HandleSensorEvent(state *internal.DeviceState, e *event.SensorEvent) {
	now := time.Now()
	state.ChipTemperature = e.ChipTemperature()
	state.Raw = e.Raw
	if e.Analog != nil {
		state.Analog = e.Analog
	}
	state.ComputeMq(e, now) // before detection: an MQ reading makes the device multi

	if state.ApplyDetection(e, false) {
		h.profiles.Save(state.Device)
	}

	// IR remotes, unrecognised devices, or a message without the device's sensor
	if !internal.HasSensorData(state.Device.SensorType, e) {
		h.handleBareTelemetry(state, e, &now)
		return
	}

	values := h.storage.Store(state.Device.ID, now, e.SeriesReadings())

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
	state.SetValues(values)

	h.dispatcher.Dispatch(&event.SensorsUpdated{DeviceId: state.Device.ID})
}

// handleBareTelemetry treats telemetry without the device's sensor data (IR remotes and
// unrecognised devices: e.g. only the ESP32 chip temperature, SetOption146 1) as a sign of
// life, on top of LWT. Whatever the message reports is still stored as series, but the live
// values are left as they are.
func (h *EventHandler) handleBareTelemetry(state *internal.DeviceState, e *event.SensorEvent, now *time.Time) {
	h.storage.Store(state.Device.ID, *now, e.SeriesReadings())
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
