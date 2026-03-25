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

func NewSensorsDailyController(db *gorm.DB, states *internal.DeviceStateStorage) *SensorsDailyController {
	return &SensorsDailyController{db: db, states: states}
}

type SensorsDailyController struct {
	db     *gorm.DB
	states *internal.DeviceStateStorage
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

	till := time.Now()
	from := till.AddDate(0, -1, 0)
	dbRecords, err := gorm.G[model.SensorHistoryModel](c.db).
		Where("device_id = ?", state.Device.ID).
		Where("date >= ?", from.Format(time.DateTime)).
		Order("id DESC").
		Find(r.Context())
	if err != nil {
		slog.Error("[sensors][daily] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	dbRecordsMap := map[string]*model.SensorHistoryModel{}
	for _, dbRecord := range dbRecords {
		date := time.Time(dbRecord.Date).Format(time.DateOnly)
		dbRecordsMap[date] = &dbRecord
	}

	records := []*DeviceSensorDailyEvent{}
	for day := from; day.After(till) == false; day = day.AddDate(0, 0, 1) {
		date := day.Format(time.DateOnly)
		dbRecord, exists := dbRecordsMap[date]
		var power *float32
		if exists {
			power = &dbRecord.Power
		} else {
			power = nil
		}

		records = append(records, &DeviceSensorDailyEvent{Date: date, PowerConsumed: power})
	}

	// ToDo: make it better, now it's just replace of last element because of the order
	records[len(records)-1].PowerConsumed = state.Today

	slog.Info("[sensors][daily] success")
	json.NewEncoder(w).Encode(records)
}
