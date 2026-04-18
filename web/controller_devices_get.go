package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"smart-home/internal"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func NewDevicesController(deviceStates *internal.DeviceStateManager) *DevicesController {
	return &DevicesController{states: deviceStates}
}

type DevicesController struct {
	states *internal.DeviceStateManager
}

func (c *DevicesController) GetAll(w http.ResponseWriter, r *http.Request) {
	events := map[uint]*Device{}
	for _, state := range c.states.GetAll() {
		event := NewDeviceEvent(state)
		if state.LastUpdate != nil {
			event.State.LastUpdate = new(state.LastUpdate.Unix())
		}

		events[state.Device.ID] = event
	}

	slog.Info("[device][get-all] success")
	json.NewEncoder(w).Encode(events)
}

func (c *DevicesController) Get(w http.ResponseWriter, r *http.Request) {
	deviceId, err := strconv.Atoi(chi.URLParam(r, "deviceId"))
	if err != nil {
		slog.Error("[device][get] error", "err", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	device := c.states.GetById(uint(deviceId))
	if device == nil {
		slog.Error("[device][get] error", "err", err)
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	normalized := NewDeviceEvent(device)
	slog.Info("[device][get] success")
	json.NewEncoder(w).Encode(normalized)
}
