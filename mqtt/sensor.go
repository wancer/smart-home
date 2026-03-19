package mqtt

import (
	"smart-home/event"
	"smart-home/model"
	"time"
)

func toModel(e *event.SensorEvent, d *model.DeviceModel) *model.SensorEventModel {
	now := time.Now()

	r := &model.SensorEventModel{}
	r.DeviceId = d.ID
	r.RealTime = now
	r.DeviceTime = time.Time(e.Time)
	r.Period = uint(e.Energy.Period)
	r.Power = uint(e.Energy.Power)
	r.ApparentPower = uint(e.Energy.ApparentPower)
	r.ReactivePower = uint(e.Energy.ReactivePower)
	r.Voltage = uint(e.Energy.Voltage)
	r.Current = e.Energy.Current

	return r
}
