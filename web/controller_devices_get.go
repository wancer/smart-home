package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"smart-home/internal"
	"smart-home/mqtt"
	"strconv"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func NewDevicesController(
	deviceStates *internal.DeviceStateManager,
	db *gorm.DB,
	storage *internal.Storage,
	consumer *mqtt.Consumer,
	pub *mqtt.Publisher,
) *DevicesController {
	return &DevicesController{states: deviceStates, db: db, storage: storage, consumer: consumer, pub: pub}
}

type DevicesController struct {
	states   *internal.DeviceStateManager
	db       *gorm.DB
	storage  *internal.Storage
	consumer *mqtt.Consumer
	pub      *mqtt.Publisher
}

func (c *DevicesController) GetAll(w http.ResponseWriter, r *http.Request) {
	sizes, err := c.storage.GetHistorySizes(r.Context())
	if err != nil {
		slog.Error("[device][get-all] history size error", "err", err)
	}

	events := map[uint]*DeviceResponse{}
	for _, state := range c.states.GetAll() {
		event := NewDeviceResponse(state)
		if state.LastUpdate != nil {
			event.State.LastUpdate = new(state.LastUpdate.Unix())
		}

		if err == nil {
			size := sizes[state.Device.ID]
			event.SensorHistory = &SensorHistorySize{
				Aggregates: size.Aggregates,
				Daily:      size.Daily,
				Archived:   size.Archived,
				Oldest:     size.Oldest,
			}
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
