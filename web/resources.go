package web

import (
	"smart-home/event"
	"smart-home/internal"
	"smart-home/model"
	"time"
)

type SensorEvent struct {
	DeviceId uint    `json:"deviceId"`
	Time     int64   `json:"time"` // ToDo: fix me
	Power    uint    `json:"power"`
	Current  float32 `json:"current"`
	Voltage  uint    `json:"voltage"`
}

func NewSensorFromEvent(e *event.SensorEvent, d *internal.Device) *SensorEvent {
	return &SensorEvent{
		DeviceId: d.ID,
		Time:     time.Time(e.Time).Unix(),
		Power:    e.Energy.Power,
		Current:  e.Energy.Current,
		Voltage:  e.Energy.Voltage,
	}
}

func NewSensorEvent(dbRecord *model.SensorEventModel) *SensorEvent {
	return &SensorEvent{
		DeviceId: dbRecord.DeviceId,
		Time:     dbRecord.RealTime.Unix(),
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
	ID uint  `json:"deviceId"`
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

func NewFirmwareConfig(c *internal.DeviceFirmware) FirmwareConfig {
	firmware := FirmwareConfig{
		Version: c.Version,
	}
	if c.BuiltAt != nil {
		formatted := c.BuiltAt.Format(time.DateTime)
		firmware.BuildAt = &formatted
	}
	return firmware
}

type LedConfig struct {
	LedState   *uint `json:"ledState"`
	LedPower   *bool `json:"ledPower"`
	LedPwmMode *bool `json:"ledPwmMode"`
	LedPwmOff  *uint `json:"ledPwmOff"`
	LedPwmOn   *uint `json:"ledPwmOn"`
}

type DeviceConfig struct {
	TelePeriod *uint          `json:"telePeriod"`
	Timezone   *string        `json:"timezone"`
	LedConfig  LedConfig      `json:"led"`
	Firmware   FirmwareConfig `json:"firmware"`
}
