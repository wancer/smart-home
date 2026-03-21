package internal

import (
	"smart-home/model"
	"time"
)

type DeviceState struct {
	Device *model.DeviceModel

	On         *bool
	LastUpdate *time.Time
	Power      *uint
	Current    *float32
	Voltage    *uint
}

type StateStorage struct {
	states map[uint]*DeviceState
}

func NewStateStorage(devices *DeviceMap) *StateStorage {
	states := map[uint]*DeviceState{}
	for _, device := range devices.GetAll() {
		states[device.ID] = &DeviceState{
			On:     nil,
			Device: device,
		}
	}

	return &StateStorage{
		states: states,
	}
}

func (d *StateStorage) GetById(id uint) *DeviceState {
	return d.states[id]
}

func (d *StateStorage) GetAll() map[uint]*DeviceState {
	return d.states
}
