package handler

import "smart-home/event"

func (h *EventHandler) InternalHandleOffline(e *event.InternalOfflineEvent) {
	state := h.states.GetById(e.DeviceId)
	if state == nil { // deleted meanwhile
		return
	}

	state.Online = false

	state.On = nil
	today := state.Today // kept: it's the daily total, not a live reading
	state.ClearReadings()
	state.Today = today

	state.ChipTemperature = nil
	state.LastTelemetry = nil
	state.Uptime = nil
	state.LoadAvg = nil

	h.dispatcher.Dispatch(&event.DeviceAvailability{DeviceId: e.DeviceId, Online: false})
}
