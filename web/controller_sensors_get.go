package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"smart-home/internal"
	"smart-home/model"
	"time"

	"gorm.io/gorm"
)

func NewSensorsController(db *gorm.DB, s *internal.Storage) *SensorsController {
	return &SensorsController{db: db, s: s}
}

type SensorsController struct {
	db *gorm.DB
	s  *internal.Storage
}

func (c *SensorsController) Get(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	dbRecords, err := gorm.G[model.SensorEventModel](c.db).Where("device_time > ?", now.AddDate(0, 0, -1).Format(time.DateTime)).Order("id DESC").Find(r.Context())
	if err != nil {
		slog.Error("[sensors][get] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	currentEvents := c.s.GetBuffer()

	events := []*SensorEvent{}
	for _, dbRecord := range dbRecords {
		record := NewSensorEvent(&dbRecord)
		events = append(events, record)
	}

	for _, currentEvent := range currentEvents {
		record := NewSensorEvent(currentEvent)
		events = append(events, record)
	}

	slog.Info("[sensors][get] success")
	json.NewEncoder(w).Encode(events)
}
