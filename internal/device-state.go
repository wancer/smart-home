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

type DeviceState struct {
	Device *Device
	Config *DeviceConfig

	On         *bool
	LastUpdate *time.Time
	Power      *uint
	Current    *float32
	Voltage    *uint
	Today      *float32 // W*h
}

type DeviceStateStorage struct {
	devices map[uint]*DeviceState
}

func NewDeviceStateStorage(devices []*model.DeviceModel) *DeviceStateStorage {
	states := map[uint]*DeviceState{}
	for _, device := range devices {
		states[device.ID] = &DeviceState{
			On: nil,
			Device: &Device{
				ID:    device.ID,
				Name:  device.Name,
				Topic: device.Topic,
			},
			Config: &DeviceConfig{},
		}
	}

	return &DeviceStateStorage{
		devices: states,
	}
}

func (d *DeviceStateStorage) GetById(id uint) *DeviceState {
	return d.devices[id]
}

func (d *DeviceStateStorage) GetAll() map[uint]*DeviceState {
	return d.devices
}

func (d *DeviceStateStorage) GetByTopic(topic string) *DeviceState {
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
