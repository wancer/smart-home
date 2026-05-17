package internal

import (
	"maps"
	"slices"
	"smart-home/model"
	"strings"
)

type DeviceMap struct {
	devices map[string]*model.Device
}

func NewDeviceMap(devices []*model.Device) *DeviceMap {
	deviceMap := map[string]*model.Device{}
	for _, dbDevice := range devices {
		deviceMap[dbDevice.Topic] = dbDevice
	}

	return &DeviceMap{devices: deviceMap}
}

func (d *DeviceMap) GetByTopic(topic string) *model.Device {
	if pos1 := strings.Index(topic, "/"); pos1 != -1 {
		pos1++
		pos2 := strings.Index(topic[pos1:], "/") + pos1
		topic = topic[pos1:pos2]
	}

	return d.devices[topic]
}

func (d *DeviceMap) GeyById(id uint) *model.Device {
	for _, device := range d.devices {
		if device.ID == id {
			return device
		}
	}
	return nil
}

func (d *DeviceMap) GetAll() []*model.Device {
	values := slices.Collect(maps.Values(d.devices))
	return values
}
