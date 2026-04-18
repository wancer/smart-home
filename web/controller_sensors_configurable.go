package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"smart-home/internal"
	"smart-home/model"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

const (
	sensorsFreq = 15 // sec
	secInMin    = 60
	minInHour   = 60
)

func NewSensorsConfigurableController(
	db *gorm.DB,
	states *internal.DeviceStateManager,
	buffer *internal.Storage,
) *SensorsConfigurableController {
	return &SensorsConfigurableController{db: db, states: states, buffer: buffer}
}

type SensorsConfigurableController struct {
	db     *gorm.DB
	states *internal.DeviceStateManager
	buffer *internal.Storage
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
	till = till.Truncate(time.Minute)
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

	var format func(*time.Time) string
	if scale > time.Hour {
		format = func(t *time.Time) string { return t.Format(time.DateOnly) }
	} else if scale == time.Hour {
		format = func(t *time.Time) string { return fmt.Sprintf("%s %02d", t.Format(time.DateOnly), t.Hour()) }
	} else {
		format = func(t *time.Time) string { return fmt.Sprintf("%02d:%02d", t.Hour(), t.Minute()) }
	}

	for _, buffered := range c.buffer.GetBuffer() {
		if buffered.DeviceId == state.Device.ID {
			dbRecords = append(dbRecords, *buffered)
		}
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
			Time:          format(&timeInStep),
			PowerConsumed: nil,
			PowerAvg:      nil,
			CurrentAvg:    nil,
			VoltageAvg:    nil,
		}

		dbRecordsMatchCount := uint(len(dbRecordsMatch))
		if dbRecordsMatchCount > 0 {
			var powerConsumed float32 = 0
			var powerAvg uint = 0
			var currentAvg float32 = 0
			var voltageAvg uint = 0
			for _, dbRecord := range dbRecordsMatch {
				powerConsumed += float32(dbRecord.Power) * sensorsFreq / secInMin / minInHour
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
