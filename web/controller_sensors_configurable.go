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

	records := c.buildRecords(scale, state, duration, w, r)

	slog.Info("[sensors][configurable] success")
	json.NewEncoder(w).Encode(records)
}

func (c *SensorsConfigurableController) buildRecords(
	scale time.Duration,
	state *internal.DeviceState,
	duration time.Duration,
	w http.ResponseWriter,
	r *http.Request,
) []DeviceSensorEvent {
	records := []DeviceSensorEvent{}

	till := time.Now()
	till = till.Truncate(time.Minute)
	from := till.Add(-duration)
	dbRecords, err := gorm.G[model.SensorEvent](c.db).
		Where("device_id = ?", state.Device.ID).
		Where("datetime(real_time) >= datetime(?)", from.UTC().Format(time.DateTime)).
		Order("id ASC").
		Find(r.Context())
	if err != nil {
		slog.Error("[sensors][configurable] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return records
	}

	for _, buffered := range c.buffer.GetBuffer() {
		if buffered.DeviceId == state.Device.ID {
			dbRecords = append(dbRecords, *buffered)
		}
	}

	prevStep := from
	for timeInStep := from; timeInStep.After(till) == false; timeInStep = timeInStep.Add(scale) {

		dbRecordsMatch := []*model.SensorEvent{}
		for _, dbRecord := range dbRecords {
			dbRecordDate := dbRecord.RealTime
			if dbRecordDate.Before(timeInStep) && dbRecordDate.After(prevStep) {
				dbRecordsMatch = append(dbRecordsMatch, &dbRecord)
			}
		}

		record := DeviceSensorEvent{
			Time: timeInStep.Unix(),

			PowerConsumed: nil,
			PowerAvg:      nil,
			CurrentAvg:    nil,
			VoltageAvg:    nil,

			CO2eAvg:        nil,
			TemperatureAvg: nil,
			HumidityAvg:    nil,
		}

		dbRecordsMatchCount := uint(len(dbRecordsMatch))
		if dbRecordsMatchCount > 0 {
			switch state.Device.SensorType {
			case model.SensorTypeEnergy:
				var powerConsumed float32 = 0
				var powerAvg uint = 0
				var currentAvg float32 = 0
				var voltageAvg uint = 0
				for _, dbRecord := range dbRecordsMatch {
					powerConsumed += float32(*dbRecord.Power) * sensorsFreq / secInMin / minInHour
					powerAvg += *dbRecord.Power
					currentAvg += *dbRecord.Current
					voltageAvg += *dbRecord.Voltage
				}
				record.PowerConsumed = &powerConsumed

				powerAvg = powerAvg / dbRecordsMatchCount
				record.PowerAvg = &powerAvg

				currentAvg = currentAvg / float32(dbRecordsMatchCount)
				record.CurrentAvg = &currentAvg

				voltageAvg = voltageAvg / dbRecordsMatchCount
				record.VoltageAvg = &voltageAvg
			case model.SensorTypeCo2:
				var temperatureAvg float32 = 0
				var co2eAvg uint = 0
				var humidityAvg float32 = 0
				for _, dbRecord := range dbRecordsMatch {
					temperatureAvg += *dbRecord.Temperature
					co2eAvg += *dbRecord.CO2e
					humidityAvg += *dbRecord.Humidity
				}

				temperatureAvg = temperatureAvg / float32(dbRecordsMatchCount)
				record.TemperatureAvg = &temperatureAvg

				co2eAvg = co2eAvg / dbRecordsMatchCount
				record.CO2eAvg = &co2eAvg

				humidityAvg = humidityAvg / float32(dbRecordsMatchCount)
				record.HumidityAvg = &humidityAvg
			case model.SensorTypeTempHumid:
				var temperatureAvg float32 = 0
				var humidityAvg float32 = 0
				for _, dbRecord := range dbRecordsMatch {
					temperatureAvg += *dbRecord.Temperature
					humidityAvg += *dbRecord.Humidity
				}

				temperatureAvg = temperatureAvg / float32(dbRecordsMatchCount)
				record.TemperatureAvg = &temperatureAvg

				humidityAvg = humidityAvg / float32(dbRecordsMatchCount)
				record.HumidityAvg = &humidityAvg
			default:
				slog.Error("UNKOWN_TYPE: " + state.Device.SensorType)
			}
		}

		records = append(records, record)

		prevStep = timeInStep
	}

	return records
}
