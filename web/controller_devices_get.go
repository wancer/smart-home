package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"smart-home/internal"
)

func NewDevicesController(deviceStates *internal.StateStorage) *DevicesController {
	return &DevicesController{deviceStates: deviceStates}
}

type DevicesController struct {
	deviceStates *internal.StateStorage
}

func (c *DevicesController) Get(w http.ResponseWriter, r *http.Request) {
	records := []*Device{}
	for id, state := range c.deviceStates.GetAll() {
		record := &Device{
			ID:   id,
			Name: state.Device.Name,
			State: &DeviceState{
				On:      state.On,
				Power:   state.Power,
				Voltage: state.Voltage,
				Current: state.Current,
			},
		}
		if state.LastUpdate != nil {
			record.State.LastUpdate = valueToPointer(state.LastUpdate.Unix())
		}

		records = append(records, record)
	}

	slog.Info("[device][get] success")
	json.NewEncoder(w).Encode(records)
}

func valueToPointer[V any](v V) *V {
	return &v
}
