package web

import (
	"smart-home/event"
	"smart-home/internal"
	"smart-home/model"
	"time"
)

type SensorEvent struct {
	DeviceId uint    `json:"deviceId"`
	RealTime int64   `json:"deviceTime"` // ToDo: fix me
	Power    uint    `json:"power"`
	Current  float32 `json:"current"`
	Voltage  uint    `json:"voltage"`
}

func NewSensorFromEvent(e *event.SensorEvent, d *internal.Device) *SensorEvent {
	return &SensorEvent{
		DeviceId: d.ID,
		RealTime: time.Time(e.Time).Unix(),
		Power:    e.Energy.Power,
		Current:  e.Energy.Current,
		Voltage:  e.Energy.Voltage,
	}
}

func NewSensorEvent(dbRecord *model.SensorEventModel) *SensorEvent {
	return &SensorEvent{
		DeviceId: dbRecord.DeviceId,
		RealTime: dbRecord.RealTime.Unix(),
		Power:    dbRecord.Power,
		Current:  dbRecord.Current,
		Voltage:  dbRecord.Voltage,
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

type WsStateEvent struct {
	ID uint  `json:"id"`
	On *bool `json:"on"`
}

type DeviceSensorEvent struct {
	Time          string   `json:"time"`
	PowerConsumed *float32 `json:"powerConsumed"`
	PowerAvg      *uint    `json:"powerAvg"`
	CurrentAvg    *float32 `json:"currentAvg"`
	VoltageAvg    *uint    `json:"voltageAvg"`
}

type DeviceSensorDailyEvent struct {
	Date          string   `json:"date"`
	PowerConsumed *float32 `json:"power"`
}

type FirmwareConfig struct {
	Version *string `json:"version"`
	BuildAt *string `json:"buildAt"`
}

type DeviceConfig struct {
	LedState   *uint          `json:"ledState"`
	LedPower   *bool          `json:"ledPower"`
	TelePeriod *uint          `json:"telePeriod"`
	LedPwmMode *bool          `json:"ledPwmMode"`
	LedPwmOff  *uint          `json:"ledPwmOff"`
	LedPwmOn   *uint          `json:"ledPwmOn"`
	Timezone   *string        `json:"timezone"`
	Firmware   FirmwareConfig `json:"firmware"`
}
