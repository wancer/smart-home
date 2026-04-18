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
	model := toModel(e, state.Device.ID, &now)

	h.storage.Store(model)
	h.storage.StoreDaily(e, state.Device.ID)

	if !state.Online {
		slog.Info(fmt.Sprintf("Getting %s online", state.Device.Name))
		state.Online = true
		h.p.PublishStates(state.Device)
	}
	state.Current = &e.Energy.Current
	state.Power = &e.Energy.Power
	state.Voltage = &e.Energy.Voltage
	state.Today = &e.Energy.Today
	state.LastUpdate = &now
}

func toModel(e *event.SensorEvent, deviceId uint, now *time.Time) *model.SensorEventModel {
	r := &model.SensorEventModel{}
	r.DeviceId = deviceId
	r.RealTime = *now
	r.DeviceTime = time.Time(e.Time)
	r.Period = e.Energy.Period
	r.Power = e.Energy.Power
	r.ApparentPower = e.Energy.ApparentPower
	r.ReactivePower = e.Energy.ReactivePower
	r.Voltage = e.Energy.Voltage
	r.Current = e.Energy.Current

	return r
}
