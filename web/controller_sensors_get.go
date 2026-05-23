package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"smart-home/internal"
	"smart-home/model"
	"strconv"
	"time"

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

func (c *SensorsController) GetMulti(w http.ResponseWriter, r *http.Request) {
	idParams := r.URL.Query()["ids"]
	if len(idParams) == 0 {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	var deviceIds []uint
	states := map[uint]*internal.DeviceState{}
	for _, param := range idParams {
		id, err := strconv.Atoi(param)
		if err != nil || id <= 0 {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		uid := uint(id)
		state := c.states.GetById(uid)
		if state == nil {
			continue
		}
		deviceIds = append(deviceIds, uid)
		states[uid] = state
	}

	result := map[uint][]*SensorEvent{}
	for _, id := range deviceIds {
		result[id] = []*SensorEvent{}
	}

	if len(deviceIds) > 0 {
		now := time.Now()
		dbRecords, err := gorm.G[model.SensorEvent](c.db).
			Where("device_id IN ?", deviceIds).
			Where("datetime(real_time) > datetime(?)", now.Add(-5*time.Minute).UTC().Format(time.DateTime)).
			Order("id ASC").
			Find(r.Context())
		if err != nil {
			slog.Error("[sensors][multi] error", "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		for _, dbRecord := range dbRecords {
			state, ok := states[dbRecord.DeviceId]
			if !ok {
				continue
			}
			record := NewSensorEvent(&dbRecord, state.Device)
			result[dbRecord.DeviceId] = append(result[dbRecord.DeviceId], record)
		}

		for _, currentEvent := range c.s.GetBuffer() {
			state, ok := states[currentEvent.DeviceId]
			if !ok {
				continue
			}
			record := NewSensorEvent(currentEvent, state.Device)
			result[currentEvent.DeviceId] = append(result[currentEvent.DeviceId], record)
		}
	}

	slog.Info("[sensors][multi] success", "count", len(deviceIds))
	json.NewEncoder(w).Encode(result)
}
