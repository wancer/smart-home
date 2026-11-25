package web

import (
	"slices"
	"smart-home/event"
	"smart-home/internal"
	"smart-home/model"
	"time"
)

// SensorEvent is the WS `sensor` event: the device's live series values of one telemetry message.
type SensorEvent struct {
	DeviceId   uint   `json:"deviceId"`
	Time       int64  `json:"time"`
	DeviceTime string `json:"deviceTime"`
	// Values are the live series values (series ID -> value).
	Values map[uint]float64 `json:"values"`
}

// NewSensorFromEvent builds a live WS event; values are the device's live series values.
func NewSensorFromEvent(e *event.SensorEvent, d *model.Device, values map[uint]float64) *SensorEvent {
	if values == nil {
		values = map[uint]float64{}
	}
	return &SensorEvent{
		DeviceId:   d.ID,
		Time:       time.Now().Unix(),
		DeviceTime: time.Time(e.Time).Format(time.DateTime),
		Values:     values,
	}
}

// SeriesSource gives a device's series metadata (internal.Storage).
type SeriesSource interface {
	DeviceSeries(deviceId uint) []model.SensorSeries
}

// SeriesResponse is one series of a device: device × sensor (source) × field, its kind (the
// physical quantity, see model.Kinds) and the user's label/hidden setting.
type SeriesResponse struct {
	Id        uint    `json:"id"`
	Source    string  `json:"source"`
	Field     string  `json:"field"`
	Kind      string  `json:"kind"`
	Unit      string  `json:"unit"`
	Precision int     `json:"precision"`
	Label     *string `json:"label"`
	Hidden    bool    `json:"hidden"`
	LiveOnly  bool    `json:"liveOnly"` // only the live value, no history (no charts)
}

func newSeriesResponse(s model.SensorSeries) SeriesResponse {
	info := model.Kinds[s.Kind]
	return SeriesResponse{
		Id: s.Id, Source: s.Source, Field: s.Field, Kind: s.Kind,
		Unit: info.Unit, Precision: info.Precision, Label: s.Label, Hidden: s.Hidden,
		LiveOnly: s.LiveOnly(),
	}
}

func newSeriesList(series []model.SensorSeries) []SeriesResponse {
	result := make([]SeriesResponse, len(series))
	for i, s := range series {
		result[i] = newSeriesResponse(s)
	}
	return result
}

func NewDeviceResponse(state *internal.DeviceState, source SeriesSource) *DeviceResponse {
	values := state.Values
	if values == nil {
		values = map[uint]float64{}
	}
	var last *int64
	if state.LastUpdate != nil {
		last = new(state.LastUpdate.Unix())
	}
	return &DeviceResponse{
		Series:          newSeriesList(source.DeviceSeries(state.Device.ID)),
		DashboardSeries: dashboardView(state.Device),
		ID:              state.Device.ID,
		Name:            state.Device.Name,
		Topic:           state.Device.Topic,
		Enabled:         state.Device.Enabled,
		Platform:        state.Device.Platform,
		SensorType:      state.Device.SensorType,
		SupportsToggle:  state.CanToggle(),
		Profile:         newDeviceProfile(state),
		OfflineTimeout:  state.Device.OfflineTimeout,
		StorageTiers:    state.Device.Tiers(),
		State: &DeviceState{
			On:         state.On,
			Relays:     state.RelayStates(),
			LastUpdate: last,
			Values:     values,
		},
	}
}

type DeviceState struct {
	On *bool `json:"on"`
	// Relays is the state of each relay (POWER1..n, null while unknown); null without relays.
	Relays     []*bool `json:"relays"`
	LastUpdate *int64  `json:"last"`
	// Values are the live series values (series ID -> value); empty while offline.
	Values map[uint]float64 `json:"values"`
}

type DeviceResponse struct {
	ID             uint           `json:"id"`
	Name           string         `json:"name"`
	Topic          string         `json:"topic"`
	Enabled        bool           `json:"enabled"`
	Platform       string         `json:"platform"`       // tasmota, esphome (configured in its YAML, no commands)
	SensorType     string         `json:"sensorType"`     // detected: energy, co2, t-h, multi, ir, light, other
	SupportsToggle bool           `json:"supportsToggle"` // has a relay or a light (template), not IR
	Profile        *DeviceProfile `json:"profile"`
	OfflineTimeout uint           `json:"offlineTimeout"` // seconds, 0 = auto
	// StorageTiers is the effective sensor data retention (the default when not set).
	StorageTiers []model.StorageTier `json:"storageTiers"`
	// DashboardSeries is what the dashboard shows, by series (chart null and values null = auto).
	DashboardSeries model.DashboardView `json:"dashboardSeries"`
	// Series are the device's sensor series (every one ever seen, also hidden ones).
	Series []SeriesResponse `json:"series"`
	State  *DeviceState     `json:"state"`

	// SensorHistory is only filled in the device list.
	SensorHistory *SensorHistorySize `json:"sensorHistory,omitempty"`
}

type SensorHistorySize struct {
	Aggregates int64  `json:"aggregates"`
	Daily      int64  `json:"daily"`
	Oldest     *int64 `json:"oldest"`
}

type WsStateEvent struct {
	ID     uint    `json:"deviceId"`
	On     *bool   `json:"on"`
	Relays []*bool `json:"relays"` // as DeviceState.Relays
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
	FirmwareCore    *string               `json:"firmwareCore"`
	FirmwareSdk     *string               `json:"firmwareSdk"`
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
	Mq        bool    `json:"mq"`       // MQ-xxx gas sensor enabled on an analog input: calibration
	Analog    bool    `json:"analog"`   // reports ANALOG values: an MQ sensor can be set up
	ChipTemp  bool    `json:"chipTemp"` // reports the ESP32 internal temperature (stored for 24 h)
}

func newDeviceProfile(state *internal.DeviceState) *DeviceProfile {
	d := state.Device
	p := &DeviceProfile{
		Template: d.Template,
		Sensors:  d.Sensors,
		Features: DeviceFeatures{
			Ir:       d.SensorType == model.SensorTypeIR,
			Energy:   state.HasEnergyMeter(),
			ChipTemp: state.HasChipTemperature(),
			Mq:       state.HasMq(),
			Analog:   slices.Contains(d.Sensors, "ANALOG"),
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
	if d.FirmwareCore != "" {
		p.FirmwareCore = new(d.FirmwareCore)
	}
	if d.FirmwareSdk != "" {
		p.FirmwareSdk = new(d.FirmwareSdk)
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
	Core            *string `json:"core"` // STATUS2 Core, e.g. "3.3.8"
	Sdk             *string `json:"sdk"`  // STATUS2 SDK, e.g. "5.5.4.260407"
	LatestVersion   *string `json:"latestVersion"`
	UpdateAvailable bool    `json:"updateAvailable"`
}

func NewFirmwareConfig(d *model.Device, latest *string) FirmwareConfig {
	if !d.IsTasmota() {
		latest = nil // the latest Tasmota release says nothing about other firmware
	}
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
	if d.FirmwareCore != "" {
		firmware.Core = new(d.FirmwareCore)
	}
	if d.FirmwareSdk != "" {
		firmware.Sdk = new(d.FirmwareSdk)
	}
	return firmware
}

type LedConfig struct {
	LedState   *uint `json:"ledState"`
	LedPower   *bool `json:"ledPower"`
	LedPwmMode *bool `json:"ledPwmMode"`
	LedPwmOff  *uint `json:"ledPwmOff"`
	LedPwmOn   *uint `json:"ledPwmOn"`

	StatusLed  bool    `json:"statusLed"` // has a status LED (Led/LedLink GPIO), or the template is unknown
	Light      *string `json:"light"`     // "rgb" (WS2812 / 3+ PWM), "dimmer" (1-2 PWM) or null
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

func dashboardView(device *model.Device) model.DashboardView {
	if device.Dashboard == nil {
		return model.DashboardView{}
	}
	return *device.Dashboard
}
