package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"smart-home/internal"
	"smart-home/model"

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
	ctx := r.Context()
	dbRecords, err := gorm.G[model.SensorEventModel](c.db).Order("id DESC").Find(ctx)
	if err != nil {
		slog.Error("[sensors][get] error", "err", err)
		http.Error(w, http.StatusText(500), 500)
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

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(events)
}
