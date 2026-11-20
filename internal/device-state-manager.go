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

	LedHardware *LedHardware // from the GPIO template; nil until the Template reply arrives
	LightPower  *bool
	Dimmer      *uint // 0..100
	Hue         *uint // 0..360
	Saturation  *uint // 0..100

	Timers [16]*TimerConfig
	Rules  [3]*RuleConfig
}

type DeviceFirmware struct {
	Version *string
	BuiltAt *time.Time
}

type DeviceState struct {
	Device   *model.Device
	Config   *DeviceConfig
	Firmware *DeviceFirmware
	Hardware *string

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

	DcVoltage *float32
	DcCurrent *float32
	DcPower   *float32

	ChipTemperature *float32   // ESP32 internal temperature (SetOption146 1)
	LastTelemetry   *time.Time // last tele/SENSOR message; IR availability falls back to LWT while nil

	Uptime  *DeviceUptime // from tele/STATE or STATUS11
	LoadAvg *uint         // Tasmota main loop load, %
}

// SetReadings replaces the live sensor values with the ones a telemetry message reported.
func (s *DeviceState) SetReadings(r *event.Readings) {
	s.Power, s.Current, s.Voltage, s.Today = r.Power, r.Current, r.Voltage, r.Today
	s.CO2, s.Temperature, s.Humidity, s.DewPoint, s.Pressure = r.CO2, r.Temperature, r.Humidity, r.DewPoint, r.Pressure
	s.DcVoltage, s.DcCurrent, s.DcPower = r.DcVoltage, r.DcCurrent, r.DcPower
}

// ClearReadings forgets the live sensor values, e.g. when the device goes offline.
func (s *DeviceState) ClearReadings() {
	s.SetReadings(&event.Readings{})
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
			Config:   &DeviceConfig{},
			Firmware: &DeviceFirmware{},
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
		Config:   &DeviceConfig{},
		Firmware: &DeviceFirmware{},
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
