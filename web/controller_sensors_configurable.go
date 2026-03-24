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

func NewSensorsConfigurableController(db *gorm.DB, states *internal.StateStorage) *SensorsConfigurableController {
	return &SensorsConfigurableController{db: db, states: states}
}

type SensorsConfigurableController struct {
	db     *gorm.DB
	states *internal.StateStorage
}

func (c *SensorsConfigurableController) Get(w http.ResponseWriter, r *http.Request) {
	deviceId, err := strconv.Atoi(chi.URLParam(r, "deviceId"))
	if err != nil {
		slog.Error("[sensors][configurable] error", "err", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	state := c.states.GetById(uint(deviceId))
	if state == nil {
		slog.Error("[sensors][configurable] error", "err", err)
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	duration, err := time.ParseDuration(chi.URLParam(r, "duration"))
	if err != nil {
		slog.Error("[sensors][configurable] error", "err", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	scale, err := time.ParseDuration(chi.URLParam(r, "scale"))
	if err != nil {
		slog.Error("[sensors][configurable] error", "err", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	till := time.Now()
	// till = till.Add(-time.Duration(time.Second * time.Second))
	from := till.Add(-duration)
	dbRecords, err := gorm.G[model.SensorEventModel](c.db).
		Where("device_id = ?", state.Device.ID).
		Where("real_time >= ?", from.Format(time.DateTime)).
		Order("id ASC").
		Find(r.Context())
	if err != nil {
		slog.Error("[sensors][configurable] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	records := []*DeviceSensorEvent{}
	prevStep := from
	for timeInStep := from; timeInStep.After(till) == false; timeInStep = timeInStep.Add(scale) {

		dbRecordsMatch := []*model.SensorEventModel{}
		for _, dbRecord := range dbRecords {
			dbRecordDate := dbRecord.RealTime
			if dbRecordDate.Before(timeInStep) && dbRecordDate.After(prevStep) {
				dbRecordsMatch = append(dbRecordsMatch, &dbRecord)
			}
		}

		record := &DeviceSensorEvent{
			Time:          timeInStep.Format(time.TimeOnly),
			PowerConsumed: nil,
			PowerAvg:      nil,
			CurrentAvg:    nil,
			VoltageAvg:    nil,
		}

		dbRecordsMatchCount := uint(len(dbRecordsMatch))
		if dbRecordsMatchCount > 0 {
			var powerConsumed uint = 0
			var powerAvg uint = 0
			var currentAvg float32 = 0
			var voltageAvg uint = 0
			for _, dbRecord := range dbRecordsMatch {
				powerConsumed += dbRecord.Period
				powerAvg += dbRecord.Power
				currentAvg += dbRecord.Current
				voltageAvg += dbRecord.Voltage
			}
			record.PowerConsumed = &powerConsumed

			powerAvg = powerAvg / dbRecordsMatchCount
			record.PowerAvg = &powerAvg

			currentAvg = currentAvg / float32(dbRecordsMatchCount)
			record.CurrentAvg = &currentAvg

			voltageAvg = voltageAvg / dbRecordsMatchCount
			record.VoltageAvg = &voltageAvg
		}

		records = append(records, record)

		prevStep = timeInStep
	}

	slog.Info("[sensors][configurable] success")
	json.NewEncoder(w).Encode(records)
}
