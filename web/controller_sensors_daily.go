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

func NewSensorsDailyController(db *gorm.DB, states *internal.DeviceStateManager) *SensorsDailyController {
	return &SensorsDailyController{db: db, states: states}
}

type SensorsDailyController struct {
	db     *gorm.DB
	states *internal.DeviceStateManager
}

func (c *SensorsDailyController) Get(w http.ResponseWriter, r *http.Request) {
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
	till := now
	from := till.AddDate(0, -1, 0)

	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		parsed, parseErr := time.Parse(time.DateOnly, fromStr)
		if parseErr != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		from = parsed
	}
	if tillStr := r.URL.Query().Get("till"); tillStr != "" {
		parsed, parseErr := time.Parse(time.DateOnly, tillStr)
		if parseErr != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		till = parsed
	}

	dbRecords, err := gorm.G[model.SensorHistory](c.db).
		Where("device_id = ?", state.Device.ID).
		Where("date(date) >= ?", from.UTC().Format(time.DateOnly)).
		Where("date(date) <= ?", till.UTC().Format(time.DateOnly)).
		Order("id DESC").
		Find(r.Context())
	if err != nil {
		slog.Error("[sensors][daily] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	dbRecordsMap := map[string]*model.SensorHistory{}
	for _, dbRecord := range dbRecords {
		date := time.Time(dbRecord.Date).Format(time.DateOnly)
		dbRecordsMap[date] = &dbRecord
	}

	todayStr := now.Format(time.DateOnly)
	records := []*DeviceSensorDailyEvent{}
	for day := from; !day.After(till); day = day.AddDate(0, 0, 1) {
		date := day.Format(time.DateOnly)
		dbRecord, exists := dbRecordsMap[date]
		var power *float32
		if exists {
			power = &dbRecord.Power
		} else {
			power = nil
		}
		if date == todayStr {
			power = state.Today
		}
		records = append(records, &DeviceSensorDailyEvent{Date: date, PowerConsumed: power})
	}

	slog.Info("[sensors][daily] success")
	json.NewEncoder(w).Encode(records)
}
