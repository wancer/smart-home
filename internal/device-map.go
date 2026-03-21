package internal

import (
	"maps"
	"slices"
	"smart-home/model"
	"strings"
)

type DeviceMap struct {
	devices map[string]*model.DeviceModel
}

func NewDeviceMap(devices []*model.DeviceModel) *DeviceMap {
	deviceMap := map[string]*model.DeviceModel{}
	for _, dbDevice := range devices {
		deviceMap[dbDevice.Topic] = dbDevice
	}

	return &DeviceMap{devices: deviceMap}
}

func (d *DeviceMap) GetByTopic(topic string) *model.DeviceModel {
	if pos1 := strings.Index(topic, "/"); pos1 != -1 {
		pos1++
		pos2 := strings.Index(topic[pos1:], "/") + pos1
		topic = topic[pos1:pos2]
	}

	return d.devices[topic]
}

func (d *DeviceMap) GeyById(id uint) *model.DeviceModel {
	for _, device := range d.devices {
		if device.ID == id {
			return device
		}
	}
	return nil
}

func (d *DeviceMap) GetAll() []*model.DeviceModel {
	values := slices.Collect(maps.Values(d.devices))
	return values
}
