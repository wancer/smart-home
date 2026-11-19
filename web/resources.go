package web

import (
	"log/slog"
	"smart-home/event"
	"smart-home/internal"
	"smart-home/model"
	"time"
)

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

	ChipTemperature *float32 `json:"chipTemperature"`
}

func NewSensorFromEvent(e *event.SensorEvent, d *model.Device) *SensorEvent {
	normalized := &SensorEvent{
		DeviceId:   d.ID,
		Time:       time.Now().Unix(),
		DeviceTime: time.Time(e.Time).Format(time.DateTime),

		ChipTemperature: e.ChipTemperature(),
	}

	switch d.SensorType {
	case model.SensorTypeEnergy:
		normalized.Power = &e.Energy.Power
		normalized.Current = &e.Energy.Current
		normalized.Voltage = &e.Energy.Voltage
	case model.SensorTypeCo2:
		normalized.CO2Dioxide = &e.Co2.CO2
		normalized.Temperature = &e.Co2.Temperature
		normalized.Humidity = &e.Co2.Humidity
	case model.SensorTypeTempHumid:
		normalized.Temperature = &e.TempHum.Temperature
		normalized.Humidity = &e.TempHum.Humidity
	case model.SensorTypeIR, model.SensorTypeOther:
		// no sensors
	default:
		slog.Error("UNKOWN_TYPE: " + d.SensorType)
	}

	return normalized
}

func NewSensorEvent(dbRecord *model.SensorEvent, d *model.Device) *SensorEvent {
	normalized := &SensorEvent{
		DeviceId: dbRecord.DeviceId,
		Time:     dbRecord.RealTime.Unix(),

		ChipTemperature: dbRecord.ChipTemperature,
	}

	switch d.SensorType {
	case model.SensorTypeEnergy:
		normalized.Power = dbRecord.Power
		normalized.Current = dbRecord.Current
		normalized.Voltage = dbRecord.Voltage
	case model.SensorTypeCo2:
		normalized.CO2Dioxide = dbRecord.CO2
		normalized.Temperature = dbRecord.Temperature
		normalized.Humidity = dbRecord.Humidity
	case model.SensorTypeTempHumid:
		normalized.Temperature = dbRecord.Temperature
		normalized.Humidity = dbRecord.Humidity
	case model.SensorTypeIR, model.SensorTypeOther:
		// no sensors
	default:
		slog.Error("UNKOWN_TYPE: " + d.SensorType)
	}

	return normalized
}

func NewSensorEventFromAggregate(agg *model.SensorAggregate, d *model.Device) *SensorEvent {
	normalized := &SensorEvent{
		DeviceId: agg.DeviceId,
		Time:     agg.BucketTime.Unix(),

		ChipTemperature: agg.ChipTemperatureAvg,
	}

	switch d.SensorType {
	case model.SensorTypeEnergy:
		if agg.PowerAvg != nil {
			p := uint(*agg.PowerAvg)
			normalized.Power = &p
		}
		normalized.Current = agg.CurrentAvg
		if agg.VoltageAvg != nil {
			v := uint(*agg.VoltageAvg)
			normalized.Voltage = &v
		}
	case model.SensorTypeCo2:
		if agg.CO2Avg != nil {
			v := uint(*agg.CO2Avg)
			normalized.CO2Dioxide = &v
		}
		normalized.Temperature = agg.TemperatureAvg
		normalized.Humidity = agg.HumidityAvg
	case model.SensorTypeTempHumid:
		normalized.Temperature = agg.TemperatureAvg
		normalized.Humidity = agg.HumidityAvg
	case model.SensorTypeIR, model.SensorTypeOther:
		// no sensors
	default:
		slog.Error("UNKOWN_TYPE: " + d.SensorType)
	}

	return normalized
}

func NewDeviceResponse(state *internal.DeviceState) *DeviceResponse {
	return &DeviceResponse{
		ID:             state.Device.ID,
		Name:           state.Device.Name,
		Topic:          state.Device.Topic,
		Enabled:        state.Device.Enabled,
		SensorType:     state.Device.SensorType,
		SupportsToggle: state.CanToggle(),
		Profile:        newDeviceProfile(state),
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

	ChipTemperature *float32 `json:"chipTemperature"`
}

type DeviceResponse struct {
	ID             uint         `json:"id"`
	Name           string       `json:"name"`
	Topic          string       `json:"topic"`
	Enabled        bool         `json:"enabled"`
	SensorType     string         `json:"sensorType"`     // detected: energy, co2, t-h, ir, other
	SupportsToggle bool           `json:"supportsToggle"` // has a relay (template), not IR
	Profile        *DeviceProfile `json:"profile"`
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

	ChipTemperatureAvg *float32 `json:"chipTemperatureAvg"`
}

type DeviceSensorDailyEvent struct {
	Date          string   `json:"date"`
	PowerConsumed *float32 `json:"power"`
}

// DeviceProfile is what the device is (stored), and what the UI can offer for it.
type DeviceProfile struct {
	Template        *model.DeviceTemplate `json:"template"` // null until the device replied
	Chip            *string               `json:"chip"`
	FirmwareVersion *string               `json:"firmwareVersion"`
	FirmwareBuiltAt *string               `json:"firmwareBuiltAt"`
	Sensors         []string              `json:"sensors"`
	ProfiledAt      *int64                `json:"profiledAt"` // unix time of the last change/refresh
	Features        DeviceFeatures        `json:"features"`
}

// DeviceFeatures is derived from the profile; template-based ones are false/null while it's unknown.
type DeviceFeatures struct {
	Relays    int     `json:"relays"`
	StatusLed bool    `json:"statusLed"`
	Light     *string `json:"light"` // "rgb", "dimmer" or null
	Ir        bool    `json:"ir"`
	Energy    bool    `json:"energy"`   // energy meter: calibration
	ChipTemp  bool    `json:"chipTemp"` // reports the ESP32 internal temperature (stored for 24 h)
}

func newDeviceProfile(state *internal.DeviceState) *DeviceProfile {
	d := state.Device
	p := &DeviceProfile{
		Template: d.Template,
		Sensors:  d.Sensors,
		Features: DeviceFeatures{
			Ir:       d.SensorType == model.SensorTypeIR,
			Energy:   d.SensorType == model.SensorTypeEnergy,
			ChipTemp: state.HasChipTemperature(),
		},
	}
	if p.Sensors == nil {
		p.Sensors = []string{}
	}
	if d.Chip != "" {
		p.Chip = new(d.Chip)
	}
	if d.FirmwareVersion != "" {
		p.FirmwareVersion = new(d.FirmwareVersion)
	}
	if d.FirmwareBuiltAt != nil {
		p.FirmwareBuiltAt = new(d.FirmwareBuiltAt.Format(time.DateTime))
	}
	if d.ProfiledAt != nil {
		p.ProfiledAt = new(d.ProfiledAt.Unix())
	}
	if hw := state.LedHardware(); hw != nil {
		p.Features.Relays = hw.Relays
		p.Features.StatusLed = hw.StatusLed
		if hw.Light != internal.LightNone {
			p.Features.Light = new(hw.Light)
		}
	}
	return p
}

type FirmwareConfig struct {
	Version         *string `json:"version"`
	BuildAt         *string `json:"buildAt"`
	LatestVersion   *string `json:"latestVersion"`
	UpdateAvailable bool    `json:"updateAvailable"`
}

func NewFirmwareConfig(d *model.Device, latest *string) FirmwareConfig {
	firmware := FirmwareConfig{LatestVersion: latest}
	if d.FirmwareVersion != "" {
		firmware.Version = new(d.FirmwareVersion)
		if latest != nil {
			firmware.UpdateAvailable = internal.IsFirmwareOutdated(d.FirmwareVersion, *latest)
		}
	}
	if d.FirmwareBuiltAt != nil {
		firmware.BuildAt = new(d.FirmwareBuiltAt.Format(time.DateTime))
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
