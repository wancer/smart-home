package web

import (
	"smart-home/event"
	"smart-home/internal"
	"smart-home/model"
	"time"
)

// SensorEvent is a live or stored reading. Fields the device doesn't report are null.
type SensorEvent struct {
	DeviceId   uint   `json:"deviceId"`
	Time       int64  `json:"time"`
	DeviceTime string `json:"deviceTime"`

	Power   *uint    `json:"power"`
	Current *float32 `json:"current"`
	Voltage *uint    `json:"voltage"`

	CO2Dioxide  *uint    `json:"co2"`
	Temperature *float32 `json:"temperature"`
	Humidity    *float32 `json:"humidity"`
	Pressure    *float32 `json:"pressure"` // hPa

	DcVoltage *float32 `json:"dcVoltage"`
	DcCurrent *float32 `json:"dcCurrent"`
	DcPower   *float32 `json:"dcPower"`

	ChipTemperature *float32 `json:"chipTemperature"`
}

func NewSensorFromEvent(e *event.SensorEvent, d *model.Device) *SensorEvent {
	r := e.Readings()
	return &SensorEvent{
		DeviceId:   d.ID,
		Time:       time.Now().Unix(),
		DeviceTime: time.Time(e.Time).Format(time.DateTime),

		Power:   r.Power,
		Current: r.Current,
		Voltage: r.Voltage,

		CO2Dioxide:  r.CO2,
		Temperature: r.Temperature,
		Humidity:    r.Humidity,
		Pressure:    r.Pressure,

		DcVoltage: r.DcVoltage,
		DcCurrent: r.DcCurrent,
		DcPower:   r.DcPower,

		ChipTemperature: e.ChipTemperature(),
	}
}

func NewSensorEvent(dbRecord *model.SensorEvent) *SensorEvent {
	return &SensorEvent{
		DeviceId: dbRecord.DeviceId,
		Time:     dbRecord.RealTime.Unix(),

		Power:   dbRecord.Power,
		Current: dbRecord.Current,
		Voltage: dbRecord.Voltage,

		CO2Dioxide:  dbRecord.CO2,
		Temperature: dbRecord.Temperature,
		Humidity:    dbRecord.Humidity,
		Pressure:    dbRecord.Pressure,

		DcVoltage: dbRecord.DcVoltage,
		DcCurrent: dbRecord.DcCurrent,
		DcPower:   dbRecord.DcPower,

		ChipTemperature: dbRecord.ChipTemperature,
	}
}

func NewSensorEventFromAggregate(agg *model.SensorAggregate) *SensorEvent {
	return &SensorEvent{
		DeviceId: agg.DeviceId,
		Time:     agg.BucketTime.Unix(),

		Power:   truncUint(agg.PowerAvg),
		Current: agg.CurrentAvg,
		Voltage: truncUint(agg.VoltageAvg),

		CO2Dioxide:  truncUint(agg.CO2Avg),
		Temperature: agg.TemperatureAvg,
		Humidity:    agg.HumidityAvg,
		Pressure:    agg.PressureAvg,

		DcVoltage: agg.DcVoltageAvg,
		DcCurrent: agg.DcCurrentAvg,
		DcPower:   agg.DcPowerAvg,

		ChipTemperature: agg.ChipTemperatureAvg,
	}
}

func truncUint(v *float32) *uint {
	if v == nil {
		return nil
	}
	return new(uint(*v))
}

func NewDeviceResponse(state *internal.DeviceState) *DeviceResponse {
	return &DeviceResponse{
		ID:             state.Device.ID,
		Name:           state.Device.Name,
		Topic:          state.Device.Topic,
		Enabled:        state.Device.Enabled,
		SensorType:     state.Device.SensorType,
		SupportsToggle: state.Device.SupportsToggle,
		StoreChipTemp:  state.Device.StoreChipTemp,
		OfflineTimeout: state.Device.OfflineTimeout,
		StorageTiers:   state.Device.Tiers(),
		State: &DeviceState{
			On: state.On,

			Power:   state.Power,
			Voltage: state.Voltage,

			Current:     state.Current,
			CO2:         state.CO2,
			Temperature: state.Temperature,
			Humidity:    state.Humidity,
			Pressure:    state.Pressure,

			DcVoltage: state.DcVoltage,
			DcCurrent: state.DcCurrent,
			DcPower:   state.DcPower,

			ChipTemperature: state.ChipTemperature,
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
	Temperature *float32 `json:"temperature"`
	Humidity    *float32 `json:"humidity"`
	Pressure    *float32 `json:"pressure"`

	DcVoltage *float32 `json:"dcVoltage"`
	DcCurrent *float32 `json:"dcCurrent"`
	DcPower   *float32 `json:"dcPower"`

	ChipTemperature *float32 `json:"chipTemperature"`
}

type DeviceResponse struct {
	ID             uint         `json:"id"`
	Name           string       `json:"name"`
	Topic          string       `json:"topic"`
	Enabled        bool         `json:"enabled"`
	SensorType     string       `json:"sensorType"`
	SupportsToggle bool         `json:"supportsToggle"`
	StoreChipTemp  bool         `json:"storeChipTemp"`
	OfflineTimeout uint         `json:"offlineTimeout"` // seconds, 0 = auto
	// StorageTiers is the effective sensor data retention (the default when not set).
	StorageTiers []model.StorageTier `json:"storageTiers"`
	State        *DeviceState        `json:"state"`

	// SensorHistory is only filled in the device list.
	SensorHistory *SensorHistorySize `json:"sensorHistory,omitempty"`
}

type SensorHistorySize struct {
	Aggregates int64  `json:"aggregates"`
	Daily      int64  `json:"daily"`
	Archived   int64  `json:"archived"`
	Oldest     *int64 `json:"oldest"`
}

type WsStateEvent struct {
	ID uint  `json:"deviceId"`
	On *bool `json:"on"`
}

// DeviceSensorEvent is one chart point. Min/max are nil for data stored before they were
// tracked.
type DeviceSensorEvent struct {
	Time int64 `json:"time"`

	PowerConsumed *float32 `json:"powerConsumed"`
	PowerAvg      *uint    `json:"powerAvg"`
	PowerMin      *uint    `json:"powerMin"`
	PowerMax      *uint    `json:"powerMax"`
	CurrentAvg    *float32 `json:"currentAvg"`
	CurrentMin    *float32 `json:"currentMin"`
	CurrentMax    *float32 `json:"currentMax"`
	VoltageAvg    *uint    `json:"voltageAvg"`
	VoltageMin    *uint    `json:"voltageMin"`
	VoltageMax    *uint    `json:"voltageMax"`

	CO2Avg         *uint    `json:"co2Avg"`
	CO2Min         *uint    `json:"co2Min"`
	CO2Max         *uint    `json:"co2Max"`
	TemperatureAvg *float32 `json:"temperatureAvg"`
	TemperatureMin *float32 `json:"temperatureMin"`
	TemperatureMax *float32 `json:"temperatureMax"`
	HumidityAvg    *float32 `json:"humidityAvg"`
	HumidityMin    *float32 `json:"humidityMin"`
	HumidityMax    *float32 `json:"humidityMax"`
	PressureAvg    *float32 `json:"pressureAvg"`
	PressureMin    *float32 `json:"pressureMin"`
	PressureMax    *float32 `json:"pressureMax"`

	DcVoltageAvg *float32 `json:"dcVoltageAvg"`
	DcVoltageMin *float32 `json:"dcVoltageMin"`
	DcVoltageMax *float32 `json:"dcVoltageMax"`
	DcCurrentAvg *float32 `json:"dcCurrentAvg"`
	DcCurrentMin *float32 `json:"dcCurrentMin"`
	DcCurrentMax *float32 `json:"dcCurrentMax"`
	DcPowerAvg   *float32 `json:"dcPowerAvg"`
	DcPowerMin   *float32 `json:"dcPowerMin"`
	DcPowerMax   *float32 `json:"dcPowerMax"`

	ChipTemperatureAvg *float32 `json:"chipTemperatureAvg"`
}

type DeviceSensorDailyEvent struct {
	Date          string   `json:"date"`
	PowerConsumed *float32 `json:"power"`
}

type FirmwareConfig struct {
	Version         *string `json:"version"`
	BuildAt         *string `json:"buildAt"`
	LatestVersion   *string `json:"latestVersion"`
	UpdateAvailable bool    `json:"updateAvailable"`
}

func NewFirmwareConfig(c *internal.DeviceFirmware, latest *string) FirmwareConfig {
	firmware := FirmwareConfig{
		Version:       c.Version,
		LatestVersion: latest,
	}
	if c.Version != nil && latest != nil {
		firmware.UpdateAvailable = internal.IsFirmwareOutdated(*c.Version, *latest)
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

	StatusLed  bool    `json:"statusLed"`  // has a status LED (Led/LedLink GPIO), or the template is unknown
	Light      *string `json:"light"`      // "rgb" (WS2812 / 3+ PWM), "dimmer" (1-2 PWM) or null
	LightPower *bool   `json:"lightPower"`
	Dimmer     *uint   `json:"dimmer"` // 0..100
	Color      *string `json:"color"`  // "#rrggbb" at full brightness
}

type RuleConfig struct {
	State int    `json:"state"`
	Once  int    `json:"once"`
	Rules string `json:"rules"`
	Free  int    `json:"free"`
}

type TimingConfig struct {
	TelePeriod     *uint   `json:"telePeriod"`
	Timezone       *string `json:"timezone"`
	UptimeSec      *uint64 `json:"uptimeSec"`      // nil until the device reports it (tele/STATE, STATUS11)
	OfflineTimeout uint    `json:"offlineTimeout"` // effective, seconds
}

type HardwareConfig struct {
	Hardware        *string        `json:"hardware"`
	ChipTemperature *float32       `json:"chipTemperature"` // ESP32 only, with SetOption146 1
	LoadAvg         *uint          `json:"loadAvg"`         // Tasmota LoadAvg, %
	Firmware        FirmwareConfig `json:"firmware"`
}

type DeviceConfig struct {
	TelePeriod *uint          `json:"telePeriod"`
	Timezone   *string        `json:"timezone"`
	LedConfig  LedConfig      `json:"led"`
	Firmware   FirmwareConfig `json:"firmware"`
	Hardware   *string        `json:"hardware"`
}
