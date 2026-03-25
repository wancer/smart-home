package mqtt

import (
	"smart-home/event"
	"smart-home/model"
	"time"
)

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
