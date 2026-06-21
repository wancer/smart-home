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

	var till time.Time
	if tillParam := r.URL.Query().Get("till"); tillParam != "" {
		tillUnix, err := strconv.ParseInt(tillParam, 10, 64)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		till = time.Unix(tillUnix, 0).Truncate(time.Minute)
	} else {
		till = time.Now().Truncate(time.Minute)
	}

	records := c.buildRecords(scale, state, duration, till, w, r)

	slog.Info("[sensors][configurable] success")
	json.NewEncoder(w).Encode(records)
}

func (c *SensorsConfigurableController) buildRecords(
	scale time.Duration,
	state *internal.DeviceState,
	duration time.Duration,
	till time.Time,
	w http.ResponseWriter,
	r *http.Request,
) []DeviceSensorEvent {
	records := []DeviceSensorEvent{}

	from := till.Add(-duration)
	dbRecords, err := gorm.G[model.SensorAggregate](c.db).
		Where("device_id = ?", state.Device.ID).
		Where("datetime(bucket_time) >= datetime(?)", from.UTC().Format(time.DateTime)).
		Where("datetime(bucket_time) < datetime(?)", till.UTC().Format(time.DateTime)).
		Order("bucket_time ASC").
		Find(r.Context())
	if err != nil {
		slog.Error("[sensors][configurable] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return records
	}

	// Merge unflushed buffer data
	dbRecords = append(dbRecords, c.buffer.GetBufferAggregates(state.Device.ID)...)

	prevStep := from
	for timeInStep := from; !timeInStep.After(till); timeInStep = timeInStep.Add(scale) {

		var matching []model.SensorAggregate
		for _, agg := range dbRecords {
			bt := agg.BucketTime.UTC()
			if (bt.Equal(prevStep) || bt.After(prevStep)) && bt.Before(timeInStep) {
				matching = append(matching, agg)
			}
		}

		record := DeviceSensorEvent{
			Time:           timeInStep.Unix(),
			PowerConsumed:  nil,
			PowerAvg:       nil,
			CurrentAvg:     nil,
			VoltageAvg:     nil,
			CO2Avg:         nil,
			CO2eAvg:        nil,
			TemperatureAvg: nil,
			HumidityAvg:    nil,
		}

		if len(matching) > 0 {
			switch state.Device.SensorType {
			case model.SensorTypeEnergy:
				var powerConsumed, powerSum, currentSum, voltageSum float32
				var totalCount uint
				for _, m := range matching {
					totalCount += m.Count
					if m.PowerConsumed != nil {
						powerConsumed += *m.PowerConsumed
					}
					if m.PowerAvg != nil {
						powerSum += *m.PowerAvg * float32(m.Count)
					}
					if m.CurrentAvg != nil {
						currentSum += *m.CurrentAvg * float32(m.Count)
					}
					if m.VoltageAvg != nil {
						voltageSum += *m.VoltageAvg * float32(m.Count)
					}
				}
				record.PowerConsumed = &powerConsumed
				if totalCount > 0 {
					n := float32(totalCount)
					pa := uint(powerSum / n)
					ca := currentSum / n
					va := uint(voltageSum / n)
					record.PowerAvg = &pa
					record.CurrentAvg = &ca
					record.VoltageAvg = &va
				}

			case model.SensorTypeCo2:
				var tempSum, humSum, co2Sum, co2eSum float32
				var totalCount uint
				for _, m := range matching {
					totalCount += m.Count
					if m.TemperatureAvg != nil {
						tempSum += *m.TemperatureAvg * float32(m.Count)
					}
					if m.CO2Avg != nil {
						co2Sum += *m.CO2Avg * float32(m.Count)
					}
					if m.CO2eAvg != nil {
						co2eSum += *m.CO2eAvg * float32(m.Count)
					}
					if m.HumidityAvg != nil {
						humSum += *m.HumidityAvg * float32(m.Count)
					}
				}
				if totalCount > 0 {
					n := float32(totalCount)
					temp := tempSum / n
					hum := humSum / n
					co2 := uint(co2Sum / n)
					co2e := uint(co2eSum / n)
					record.TemperatureAvg = &temp
					record.HumidityAvg = &hum
					record.CO2Avg = &co2
					record.CO2eAvg = &co2e
				}

			case model.SensorTypeTempHumid:
				var tempSum, humSum float32
				var totalCount uint
				for _, m := range matching {
					totalCount += m.Count
					if m.TemperatureAvg != nil {
						tempSum += *m.TemperatureAvg * float32(m.Count)
					}
					if m.HumidityAvg != nil {
						humSum += *m.HumidityAvg * float32(m.Count)
					}
				}
				if totalCount > 0 {
					n := float32(totalCount)
					temp := tempSum / n
					hum := humSum / n
					record.TemperatureAvg = &temp
					record.HumidityAvg = &hum
				}

			default:
				slog.Error("UNKOWN_TYPE: " + state.Device.SensorType)
			}
		}

		records = append(records, record)
		prevStep = timeInStep
	}

	return records
}
