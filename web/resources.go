package web

import (
	"smart-home/internal"
	"smart-home/model"
)

type SensorEvent struct {
	DeviceId      uint    `json:"deviceId"`
	RealTime      int64   `json:"deviceTime"` // ToDo: fix me
	PowerConsumed uint    `json:"period"`
	Power         uint    `json:"power"`
	Current       float32 `json:"current"`
	Voltage       uint    `json:"voltage"`
}

func NewSensorEvent(dbRecord *model.SensorEventModel) *SensorEvent {
	return &SensorEvent{
		DeviceId:      dbRecord.DeviceId,
		RealTime:      dbRecord.RealTime.Unix(),
		PowerConsumed: dbRecord.Period,
		Power:         dbRecord.Power,
		Current:       dbRecord.Current,
		Voltage:       dbRecord.Voltage,
	}
}

func NewDeviceEvent(state *internal.DeviceState) *Device {
	return &Device{
		ID:   state.Device.ID,
		Name: state.Device.Name,
		State: &DeviceState{
			On:      state.On,
			Power:   state.Power,
			Voltage: state.Voltage,
			Current: state.Current,
		},
	}
}

type DeviceState struct {
	On         *bool    `json:"on"`
	LastUpdate *int64   `json:"last"`
	Power      *uint    `json:"power"`
	Current    *float32 `json:"current"`
	Voltage    *uint    `json:"voltage"`
}

type Device struct {
	ID    uint         `json:"id"`
	Name  string       `json:"name"`
	State *DeviceState `json:"state"`
}

type DeviceSensorEvent struct {
	Time          string   `json:"time"`
	PowerConsumed *uint    `json:"powerConsumed"`
	PowerAvg      *uint    `json:"powerAvg"`
	CurrentAvg    *float32 `json:"currentAvg"`
	VoltageAvg    *uint    `json:"voltageAvg"`
}

type DeviceSensorDailyEvent struct {
	Date          string   `json:"date"`
	PowerConsumed *float32 `json:"power"`
}
