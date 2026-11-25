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

	LightPower *bool
	Dimmer     *uint // 0..100
	Hue        *uint // 0..360
	Saturation *uint // 0..100

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
	// relays is the last known state of each POWER<n> output (1-based) while online (see RelayStates).
	relays map[int]bool

	Today   *float32 // kWh consumed today (ENERGY.Today); kept while offline
	Voltage *uint    // live AC voltage (ENERGY.Voltage), for power calibration (PowerSet); nil while offline

	// Mq is the last MQ-xxx gas sensor reading (see ComputeMq), MqAt when it came; nil while offline
	Mq        *event.MqReading
	MqAt      time.Time
	mqSamples []mqSample // corrected Rs of the last minute
	// Analog is the last raw ANALOG object (A1, ...), to offer the MQ sensor setup
	Analog map[string]float32

	// Values are the live series values of the last telemetry, keyed by sensor_series ID (series
	// metadata: Storage.DeviceSeries); nil while offline.
	Values map[uint]float64

	ChipTemperature *float32   // ESP32 internal temperature (SetOption146 1)
	LastTelemetry   *time.Time // last tele/SENSOR message; IR availability falls back to LWT while nil
	// TelemetryInterval is the time between the last two tele/SENSOR messages while online: the
	// automatic offline timeout of devices that never report a TelePeriod (ESPHome).
	TelemetryInterval time.Duration
	lastSensorAt      time.Time // last tele/SENSOR message (LastTelemetry also moves on tele/STATE)

	Uptime  *DeviceUptime // from tele/STATE or STATUS11
	LoadAvg *uint         // Tasmota main loop load, %
	// RestartReason of the last boot (STATUS1), polled when the device comes online; nil while offline
	RestartReason *string
}

// MarkTelemetry records a tele/SENSOR message at now (LastTelemetry, TelemetryInterval).
func (s *DeviceState) MarkTelemetry(now *time.Time) {
	if s.Online && !s.lastSensorAt.IsZero() {
		s.TelemetryInterval = now.Sub(s.lastSensorAt)
	}
	s.lastSensorAt = *now
	s.LastTelemetry = now
}

// SetReadings keeps what a telemetry message reports besides the series values: today's energy
// total and the AC voltage (power calibration).
func (s *DeviceState) SetReadings(r *event.Readings) {
	s.Today, s.Voltage = r.Today, r.Voltage
}

// SetValues replaces the live series values with the ones a telemetry message reported.
func (s *DeviceState) SetValues(values map[uint]float64) {
	s.Values = values
}

// ClearReadings forgets the live sensor values, e.g. when the device goes offline.
func (s *DeviceState) ClearReadings() {
	s.SetReadings(&event.Readings{})
	s.ClearMq()
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
			Online: false,
			On:     nil,
			Device: device,
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
		Online: false,
		On:     nil,
		Device: device,
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
