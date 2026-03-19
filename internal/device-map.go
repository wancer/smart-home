package internal

import (
	"maps"
	"slices"
	"smart-home/model"
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
	return d.devices[topic]
}

func (d *DeviceMap) GetAll() []*model.DeviceModel {
	values := slices.Collect(maps.Values(d.devices))
	return values
}
