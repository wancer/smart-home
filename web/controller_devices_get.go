package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"smart-home/model"

	"gorm.io/gorm"
)

func NewDevicesController(db *gorm.DB) *DevicesController {
	return &DevicesController{db: db}
}

type DevicesController struct {
	db *gorm.DB
}

func (c *DevicesController) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dbRecords, err := gorm.G[model.DeviceModel](c.db).Find(ctx)
	if err != nil {
		slog.Error("[device][get] error", "err", err)
		http.Error(w, http.StatusText(500), 500)
		return
	}

	records := []Device{}
	for _, dbRecord := range dbRecords {
		record := Device{
			ID:   dbRecord.ID,
			Name: dbRecord.Name,
		}
		records = append(records, record)
	}

	slog.Info("[device][get] success")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(records)
}
