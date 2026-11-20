package handler

import "smart-home/event"

func (h *EventHandler) InternalHandleOffline(e *event.InternalOfflineEvent) {
	state := h.states.GetById(e.DeviceId)
	if state == nil { // deleted meanwhile
		return
	}

	state.Online = false

	state.On = nil
	state.Current = nil
	state.Voltage = nil
	state.Power = nil

	state.CO2 = nil
	state.Temperature = nil
	state.Humidity = nil
	state.DewPoint = nil

	state.ChipTemperature = nil
	state.LastTelemetry = nil
	state.Uptime = nil
	state.LoadAvg = nil
	state.RestartReason = nil

	h.dispatcher.Dispatch(&event.DeviceAvailability{DeviceId: e.DeviceId, Online: false})
}
