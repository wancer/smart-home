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

	CO2Dioxide  *uint    `json:"co2"`
	CO2E        *uint    `json:"co2e"`
	Temperature *float32 `json:"temperature"`
	Humidity    *float32 `json:"humidity"`
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
		normalized.CO2E = &e.Co2.CO2E
		normalized.CO2Dioxide = &e.Co2.CO2
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

func NewSensorEvent(dbRecord *model.SensorEvent, d *model.Device) *SensorEvent {
	normalized := &SensorEvent{
		DeviceId: dbRecord.DeviceId,
		Time:     dbRecord.RealTime.Unix(),
	}

	switch d.SensorType {
	case model.SensorTypeEnergy:
		normalized.Power = dbRecord.Power
		normalized.Current = dbRecord.Current
		normalized.Voltage = dbRecord.Voltage
	case model.SensorTypeCo2:
		normalized.CO2E = dbRecord.CO2e
		normalized.CO2Dioxide = dbRecord.CO2
		normalized.Temperature = dbRecord.Temperature
		normalized.Humidity = dbRecord.Humidity
	case model.SensorTypeTempHumid:
		normalized.Temperature = dbRecord.Temperature
		normalized.Humidity = dbRecord.Humidity
	default:
		slog.Error("UNKOWN_TYPE: " + d.SensorType)
	}

	return normalized
}

func NewDeviceResponse(state *internal.DeviceState) *DeviceResponse {
	return &DeviceResponse{
		ID:             state.Device.ID,
		Name:           state.Device.Name,
		SensorType:     state.Device.SensorType,
		SupportsToggle: state.Device.SupportsToggle,
		State: &DeviceState{
			On: state.On,

			Power:   state.Power,
			Voltage: state.Voltage,

			Current:     state.Current,
			CO2:         state.CO2,
			CO2E:        state.CO2E,
			Temperature: state.Temperature,
			Humidity:    state.Humidity,
		},
	}
}

type DeviceState struct {
	On         *bool  `json:"on"`
	LastUpdate *int64 `json:"last"`

	Power   *uint    `json:"power"`
	Current *float32 `json:"current"`
	Voltage *uint    `json:"voltage"`

	CO2         *uint    `json:"co2"`
	CO2E        *uint    `json:"co2e"`
	Temperature *float32 `json:"temperature"`
	Humidity    *float32 `json:"humidity"`
}

type DeviceResponse struct {
	ID             uint         `json:"id"`
	Name           string       `json:"name"`
	SensorType     string       `json:"sensorType"`
	SupportsToggle bool         `json:"supportsToggle"`
	State          *DeviceState `json:"state"`
}

type WsStateEvent struct {
	ID uint  `json:"deviceId"`
	On *bool `json:"on"`
}

type DeviceSensorEvent struct {
	Time string `json:"time"`

	PowerConsumed *float32 `json:"powerConsumed"`
	PowerAvg      *uint    `json:"powerAvg"`
	CurrentAvg    *float32 `json:"currentAvg"`
	VoltageAvg    *uint    `json:"voltageAvg"`

	CO2eAvg        *uint    `json:"co2eAvg"`
	TemperatureAvg *float32 `json:"temperatureAvg"`
	HumidityAvg    *float32 `json:"humidityAvg"`
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
