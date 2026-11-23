package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"smart-home/automation"
	"smart-home/internal"
	"smart-home/mqtt"

	"gorm.io/gorm"
)

func NewDevicesController(
	deviceStates *internal.DeviceStateManager,
	db *gorm.DB,
	storage *internal.Storage,
	consumer *mqtt.Consumer,
	pub *mqtt.Publisher,
	notifier *automation.Notifier,
) *DevicesController {
	return &DevicesController{states: deviceStates, db: db, storage: storage, consumer: consumer, pub: pub, notifier: notifier}
}

type DevicesController struct {
	states   *internal.DeviceStateManager
	db       *gorm.DB
	storage  *internal.Storage
	consumer *mqtt.Consumer
	pub      *mqtt.Publisher
	notifier *automation.Notifier
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
				Oldest:     size.Oldest,
			}
		}

		events[state.Device.ID] = event
	}

	slog.Info("[device][get-all] success")
	json.NewEncoder(w).Encode(events)
}

func (c *DevicesController) Get(w http.ResponseWriter, r *http.Request) {
	device := deviceState(w, r, c.states)
	if device == nil {
		return
	}

	normalized := NewDeviceResponse(device)
	slog.Info("[device][get] success")
	json.NewEncoder(w).Encode(normalized)
}
