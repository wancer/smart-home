package handler

import (
	"smart-home/event"
	"smart-home/internal"
	"time"
)

// HandleTeleState keeps uptime and load average. tele/STATE comes every TelePeriod,
// so it also keeps an online device alive, but never brings one online: that is
// left to sensor telemetry (or LWT for IR remotes).
func (h *EventHandler) HandleTeleState(state *internal.DeviceState, e *event.TeleState) {
	now := time.Now()
	if e.UptimeSec != nil {
		state.Uptime = &internal.DeviceUptime{Seconds: *e.UptimeSec, At: now}
	}
	if e.LoadAvg != nil {
		state.LoadAvg = new(*e.LoadAvg)
	}

	if !state.Online {
		return
	}
	state.LastUpdate = &now
	if state.LastTelemetry != nil {
		state.LastTelemetry = &now
	}
}
