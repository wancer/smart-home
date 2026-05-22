package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"smart-home/internal"
	"smart-home/model"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func NewSensorsController(db *gorm.DB, s *internal.Storage, states *internal.DeviceStateManager) *SensorsController {
	return &SensorsController{db: db, s: s, states: states}
}

type SensorsController struct {
	db     *gorm.DB
	s      *internal.Storage
	states *internal.DeviceStateManager
}

func (c *SensorsController) Get(w http.ResponseWriter, r *http.Request) {
	deviceId, err := strconv.Atoi(chi.URLParam(r, "deviceId"))
	if err != nil {
		slog.Error("[sensors][daily] error", "err", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	state := c.states.GetById(uint(deviceId))
	if state == nil {
		slog.Error("[sensors][daily] error", "err", err)
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	now := time.Now()
	dbRecords, err := gorm.G[model.SensorEvent](c.db).Where("device_id = ?", state.Device.ID).Where("datetime(real_time) > datetime(?)", now.Add(-5*time.Minute).UTC().Format(time.DateTime)).Order("id DESC").Find(r.Context())
	if err != nil {
		slog.Error("[sensors][get] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	currentEvents := c.s.GetBuffer()

	events := []*SensorEvent{}
	for _, dbRecord := range dbRecords {
		record := NewSensorEvent(&dbRecord, state.Device)
		events = append(events, record)
	}

	for _, currentEvent := range currentEvents {
		record := NewSensorEvent(currentEvent, state.Device)
		events = append(events, record)
	}

	slog.Info("[sensors][get] success")
	json.NewEncoder(w).Encode(events)
}
