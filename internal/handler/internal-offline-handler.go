package handler

import "smart-home/event"

func (h *EventHandler) InternalHandleOffline(e *event.InternalOfflineEvent) {
	state := h.states.GetById(e.DeviceId)

	state.Online = false

	state.On = nil
	state.Current = nil
	state.Voltage = nil
	state.Power = nil

	state.On = nil
	state.Current = nil
	state.Voltage = nil
	state.Power = nil

	state.CO2 = nil
	state.CO2E = nil
	state.Temperature = nil
	state.Humidity = nil
	state.DewPoint = nil
}
