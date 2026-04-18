package internal

import (
	"smart-home/model"
	"strings"
	"time"
)

type Device struct {
	ID    uint
	Name  string
	Topic string
}

type TimeSwitch struct {
	Day        uint
	Hemisphere uint
	Hour       uint
	Month      uint
	Offset     uint
	Week       uint
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
}

type DeviceFirmware struct {
	Version *string
	BuiltAt *time.Time
}

type DeviceState struct {
	Device   *Device
	Config   *DeviceConfig
	Firmware *DeviceFirmware

	Online     bool
	On         *bool
	LastUpdate *time.Time
	Power      *uint
	Current    *float32
	Voltage    *uint
	Today      *float32 // W*h
}

type DeviceStateManager struct {
	devices map[uint]*DeviceState
}

func NewDeviceStateManager(devices []*model.DeviceModel) *DeviceStateManager {
	states := map[uint]*DeviceState{}
	for _, device := range devices {
		states[device.ID] = &DeviceState{
			Online: false,
			On:     nil,
			Device: &Device{
				ID:    device.ID,
				Name:  device.Name,
				Topic: device.Topic,
			},
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
