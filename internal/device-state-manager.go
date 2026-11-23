package internal

import (
	"smart-home/event"
	"smart-home/model"
	"strings"
	"time"
)

type TimeSwitch struct {
	Day        uint
	Hemisphere uint
	Hour       uint
	Month      uint
	Offset     uint
	Week       uint
}

type TimerConfig struct {
	Enable int
	Mode   int
	Time   string
	Window int
	Days   string
	Repeat int
	Output int
	Action int
}

type RuleConfig struct {
	State int
	Once  int
	Rules string
	Free  int
}

type DeviceConfig struct {
	LedState   *uint
	LedPower   *bool
	TelePeriod *uint
	Timezone   *string
	TimeStd    *TimeSwitch
	TimeDst    *TimeSwitch
	LedPwmMode *bool
	LedPwmOff  *uint
	LedPwmOn   *uint

	LightPower  *bool
	Dimmer      *uint // 0..100
	Hue         *uint // 0..360
	Saturation  *uint // 0..100

	Timers [16]*TimerConfig
	Rules  [3]*RuleConfig
}

type DeviceState struct {
	Device *model.Device // also holds the profile (template, chip, firmware, sensors)
	Config *DeviceConfig

	// ProfileRefresh is set while a manual profile refresh waits for the replies: the next
	// STATUS10 may then change the sensor type in any direction, not only up (see ApplyDetection).
	ProfileRefresh bool

	Online     bool
	On         *bool
	LastUpdate *time.Time

	Power   *uint
	Current *float32
	Voltage *uint
	Today   *float32 // W*h

	CO2         *uint
	Temperature *float32
	Humidity    *float32
	DewPoint    *float32
	Pressure    *float32 // hPa
	GasRatio    *float32 // MQ-xxx gas level vs. calibration air
	Illuminance *float32 // lx
	ColorTemp   *float32 // K

	DcVoltage *float32
	DcCurrent *float32
	DcPower   *float32

	// Mq is the last MQ-xxx gas sensor reading (see ComputeMq), MqAt when it came; nil while offline
	Mq        *event.MqReading
	MqAt      time.Time
	mqSamples []mqSample // corrected Rs of the last minute
	// Analog is the last raw ANALOG object (A1, ...), to offer the MQ sensor setup
	Analog map[string]float32

	// Raw is every numeric sensor field of the last telemetry (event.SensorEvent.Raw); nil while offline
	Raw map[string]map[string]float64
	// Values are the live series values of the last telemetry, keyed by sensor_series ID (series
	// metadata: Storage.DeviceSeries); nil while offline. The fixed fields above (Power, ...)
	// are kept for the current API until the clients use series.
	Values map[uint]float64

	ChipTemperature *float32   // ESP32 internal temperature (SetOption146 1)
	LastTelemetry   *time.Time // last tele/SENSOR message; IR availability falls back to LWT while nil

	Uptime  *DeviceUptime // from tele/STATE or STATUS11
	LoadAvg *uint         // Tasmota main loop load, %
	// RestartReason of the last boot (STATUS1), polled when the device comes online; nil while offline
	RestartReason *string
}

// SetReadings replaces the live sensor values with the ones a telemetry message reported.
func (s *DeviceState) SetReadings(r *event.Readings) {
	s.Power, s.Current, s.Voltage, s.Today = r.Power, r.Current, r.Voltage, r.Today
	s.CO2, s.Temperature, s.Humidity, s.DewPoint, s.Pressure = r.CO2, r.Temperature, r.Humidity, r.DewPoint, r.Pressure
	s.DcVoltage, s.DcCurrent, s.DcPower = r.DcVoltage, r.DcCurrent, r.DcPower
	s.GasRatio = r.GasRatio
	s.Illuminance, s.ColorTemp = r.Illuminance, r.ColorTemp
}

// SetValues replaces the live series values with the ones a telemetry message reported.
func (s *DeviceState) SetValues(values map[uint]float64) {
	s.Values = values
}

// ClearReadings forgets the live sensor values, e.g. when the device goes offline.
func (s *DeviceState) ClearReadings() {
	s.SetReadings(&event.Readings{})
	s.ClearMq()
	s.Raw = nil
	s.Values = nil
}

type DeviceUptime struct {
	Seconds uint64    // device uptime when reported
	At      time.Time // when it was reported
}

// Current extrapolates the reported uptime to now.
func (u *DeviceUptime) Current() uint64 {
	return u.Seconds + uint64(max(0, time.Since(u.At)/time.Second))
}

type DeviceStateManager struct {
	devices map[uint]*DeviceState
}

func NewDeviceStateManager(devices []*model.Device) *DeviceStateManager {
	states := map[uint]*DeviceState{}
	for _, device := range devices {
		states[device.ID] = &DeviceState{
			Online:   false,
			On:       nil,
			Device:   device,
			Config: &DeviceConfig{},
		}
	}

	return &DeviceStateManager{
		devices: states,
	}
}

func (d *DeviceStateManager) GetById(id uint) *DeviceState {
	return d.devices[id]
}

func (d *DeviceStateManager) GetAll() map[uint]*DeviceState {
	return d.devices
}

func (d *DeviceStateManager) Add(device *model.Device) {
	d.devices[device.ID] = &DeviceState{
		Online:   false,
		On:       nil,
		Device:   device,
		Config: &DeviceConfig{},
	}
}

func (d *DeviceStateManager) Delete(id uint) {
	delete(d.devices, id)
}

func (d *DeviceStateManager) GetByTopic(topic string) *DeviceState {
	if pos1 := strings.Index(topic, "/"); pos1 != -1 {
		pos1++
		pos2 := strings.Index(topic[pos1:], "/") + pos1
		topic = topic[pos1:pos2]
	}

	for _, device := range d.devices {
		if device.Device.Topic == topic {
			return device
		}
	}

	return nil
}
