package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"smart-home/internal"
	"strconv"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func NewDevicesController(deviceStates *internal.DeviceStateManager, db *gorm.DB) *DevicesController {
	return &DevicesController{states: deviceStates, db: db}
}

type DevicesController struct {
	states *internal.DeviceStateManager
	db     *gorm.DB
}

func (c *DevicesController) GetAll(w http.ResponseWriter, r *http.Request) {
	events := map[uint]*DeviceResponse{}
	for _, state := range c.states.GetAll() {
		event := NewDeviceResponse(state)
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

	normalized := NewDeviceResponse(device)
	slog.Info("[device][get] success")
	json.NewEncoder(w).Encode(normalized)
}
