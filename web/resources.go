package web

import (
	"log/slog"
	"smart-home/event"
	"smart-home/internal"
	"smart-home/model"
	"time"
)

type SensorEvent struct {
	DeviceId uint  `json:"deviceId"`
	Time     int64 `json:"time"`

	Power   *uint    `json:"power"`
	Current *float32 `json:"current"`
	Voltage *uint    `json:"voltage"`

	CarbonDioxide *uint    `json:"CarbonDioxide"`
	CO2           *uint    `json:"co2"`
	Temperature   *float32 `json:"temperature"`
	Humidity      *float32 `json:"humidity"`
}

func NewSensorFromEvent(e *event.SensorEvent, d *model.Device) *SensorEvent {
	normalized := &SensorEvent{
		DeviceId: d.ID,
		Time:     time.Time(e.Time).Unix(),
	}

	switch d.SensorType {
	case model.SensorTypeEnergy:
		normalized.Power = &e.Energy.Power
		normalized.Current = &e.Energy.Current
		normalized.Voltage = &e.Energy.Voltage
	case model.SensorTypeCo2:
		normalized.CO2 = &e.Co2.CO2
		normalized.CarbonDioxide = &e.Co2.CarbonDioxide
		normalized.Temperature = &e.Co2.Temperature
		normalized.Humidity = &e.Co2.Humidity
	case model.SensorTypeTempHumid:
		normalized.Temperature = &e.TempHum.Temperature
		normalized.Humidity = &e.TempHum.Humidity
	default:
		slog.Error("UNKOWN_TYPE: " + d.SensorType)
	}

	return normalized
}

func NewSensorEvent(dbRecord *model.SensorEvent) *SensorEvent {
	return &SensorEvent{
		DeviceId: dbRecord.DeviceId,
		Time:     dbRecord.RealTime.Unix(),
		Power:    dbRecord.Power,
		Current:  dbRecord.Current,
		Voltage:  dbRecord.Voltage,
	}
}

func NewDeviceResponse(state *internal.DeviceState) *DeviceResponse {
	return &DeviceResponse{
		ID:   state.Device.ID,
		Name: state.Device.Name,
		State: &DeviceState{
			On:            state.On,
			Power:         state.Power,
			Voltage:       state.Voltage,
			Current:       state.Current,
			CarbonDioxide: state.CarbonDioxide,
			CO2:           state.CO2,
			Temperature:   state.Temperature,
			Humidity:      state.Humidity,
		},
	}
}

type DeviceState struct {
	On         *bool  `json:"on"`
	LastUpdate *int64 `json:"last"`

	Power   *uint    `json:"power"`
	Current *float32 `json:"current"`
	Voltage *uint    `json:"voltage"`

	CarbonDioxide *uint    `json:"CarbonDioxide"`
	CO2           *uint    `json:"co2"`
	Temperature   *float32 `json:"temperature"`
	Humidity      *float32 `json:"humidity"`
}

type DeviceResponse struct {
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
	Hardware   *string        `json:"hardware"`
}
