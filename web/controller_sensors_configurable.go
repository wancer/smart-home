package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
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
	state := deviceState(w, r, c.states)
	if state == nil {
		return
	}
	duration, err := time.ParseDuration(chi.URLParam(r, "duration"))
	if err != nil {
		slog.Error("[sensors][configurable] error", "err", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	scaleParam := chi.URLParam(r, "scale")
	var scale time.Duration
	if scaleParam != rawScale {
		scale, err = time.ParseDuration(scaleParam)
		if err != nil {
			slog.Error("[sensors][configurable] error", "err", err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
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

	var records []DeviceSensorEvent
	if scaleParam == rawScale {
		if duration <= 0 || duration > maxRawDuration {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		records, err = c.rawRecords(r.Context(), state, duration, till)
	} else {
		if scale <= 0 || duration <= 0 || duration/scale > maxChartPoints {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		records, err = c.buildRecords(r.Context(), state, duration, scale, till)
	}
	if err != nil {
		slog.Error("[sensors][configurable] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	slog.Info("[sensors][configurable] success")
	json.NewEncoder(w).Encode(records)
}

// maxChartPoints caps duration/scale, e.g. 24h at a 15s step is 5760 points.
const maxChartPoints = 10_000

// rawScale returns every stored row as its own point (raw readings at their own time,
// coarser buckets at their start) instead of fixed steps. maxRawDuration caps its range.
const (
	rawScale       = "raw"
	maxRawDuration = 24 * time.Hour
)

// rawRecords returns the rows in (till-duration, till] by time, one point per bucket time
// (all series of the device). till is truncated to the minute, so the range runs a minute
// past it to include the latest readings.
func (c *SensorsConfigurableController) rawRecords(
	ctx context.Context,
	state *internal.DeviceState,
	duration time.Duration,
	till time.Time,
) ([]DeviceSensorEvent, error) {
	rows, err := c.buffer.LoadValues(ctx, state.Device.ID, till.Add(-duration), till.Add(time.Minute))
	if err != nil {
		return nil, err
	}
	preferred := c.buffer.PreferredSeries(state.Device.ID)

	// a buffered reading can share its second with a stored one: valuesByTime merges them
	points := valuesByTime(rows)
	records := make([]DeviceSensorEvent, 0, len(points))
	for _, p := range points {
		records = append(records, newDeviceSensorEvent(p.at, preferred, p.values))
	}
	return records, nil
}

// buildRecords returns one point per scale step. Points are aligned to the clock
// (xx:00, xx:05, xx:10 for a 5m scale); each is labelled with its bucket start and covers
// [t, t+scale), matching the frontend's live WebSocket buckets (floor(time / scale) * scale).
// The last point is the in-progress bucket.
//
// When the range holds buckets coarser than scale (see model.StorageTier), the step is
// raised to the coarsest one, so the chart doesn't alternate between data and gaps. The
// step actually used shows in the points' times.
func (c *SensorsConfigurableController) buildRecords(
	ctx context.Context,
	state *internal.DeviceState,
	duration time.Duration,
	scale time.Duration,
	till time.Time,
) ([]DeviceSensorEvent, error) {
	deviceId := state.Device.ID
	from, end := chartRange(till, duration, scale)
	rows, err := c.buffer.LoadValues(ctx, deviceId, from, end)
	if err != nil {
		return nil, err
	}

	if coarsest := coarsestBucket(rows); coarsest > scale {
		scale = coarsest
		wider, newEnd := chartRange(till, duration, scale)
		end = newEnd
		if wider.Before(from) {
			from = wider
			if rows, err = c.buffer.LoadValues(ctx, deviceId, from, end); err != nil {
				return nil, err
			}
		}
	}

	// per step: series ID -> merged value
	points := make([]map[uint]*model.SensorValue, int(end.Sub(from)/scale))
	for i := range rows {
		bt := rows[i].BucketTime.UTC()
		if bt.Before(from) || !bt.Before(end) {
			continue
		}
		idx := int(bt.Sub(from) / scale)
		if points[idx] == nil {
			points[idx] = map[uint]*model.SensorValue{}
		}
		if existing, ok := points[idx][rows[i].SeriesId]; ok {
			internal.MergeValue(existing, &rows[i])
		} else {
			v := rows[i]
			points[idx][v.SeriesId] = &v
		}
	}

	preferred := c.buffer.PreferredSeries(deviceId)
	records := make([]DeviceSensorEvent, len(points))
	for i, point := range points {
		at := from.Add(time.Duration(i) * scale)
		records[i] = newDeviceSensorEvent(at, preferred, point)
	}
	return records, nil
}

func chartRange(till time.Time, duration, scale time.Duration) (time.Time, time.Time) {
	from := till.Add(-duration).Truncate(scale)
	end := till.Truncate(scale)
	if end.Before(till) {
		end = end.Add(scale)
	}
	return from, end
}

func coarsestBucket(rows []model.SensorValue) time.Duration {
	var coarsest uint
	for _, row := range rows {
		coarsest = max(coarsest, row.BucketSeconds)
	}
	return time.Duration(coarsest) * time.Second
}

// newDeviceSensorEvent maps a merged step (series ID -> value) to a chart point: each fixed
// field takes the first preferred series of its kind with a value. values is nil for an
// empty step.
func newDeviceSensorEvent(at time.Time, preferred map[string][]uint, values map[uint]*model.SensorValue) DeviceSensorEvent {
	record := DeviceSensorEvent{Time: at.Unix()}
	if values == nil {
		return record
	}
	pick := func(kind string) *model.SensorValue {
		v, _ := internal.PickValue(preferred, kind, values)
		return v
	}
	f32 := func(v *model.SensorValue) (avg, lo, hi *float32) {
		if v == nil {
			return nil, nil, nil
		}
		return new(float32(v.Avg)), new(float32(v.Min)), new(float32(v.Max))
	}
	u := func(v *model.SensorValue) (avg, lo, hi *uint) {
		a, l, h := f32(v)
		return toUint(a), toUint(l), toUint(h)
	}

	if power := pick(model.KindPower); power != nil {
		record.PowerConsumed = new(float32(0))
		if power.Sum != nil {
			record.PowerConsumed = new(float32(*power.Sum))
		}
		record.PowerAvg, record.PowerMin, record.PowerMax = u(power)
	}
	record.CurrentAvg, record.CurrentMin, record.CurrentMax = f32(pick(model.KindCurrent))
	record.VoltageAvg, record.VoltageMin, record.VoltageMax = u(pick(model.KindVoltage))

	record.CO2Avg, record.CO2Min, record.CO2Max = u(pick(model.KindCo2))
	record.TemperatureAvg, record.TemperatureMin, record.TemperatureMax = f32(pick(model.KindTemperature))
	record.HumidityAvg, record.HumidityMin, record.HumidityMax = f32(pick(model.KindHumidity))
	record.PressureAvg, record.PressureMin, record.PressureMax = f32(pick(model.KindPressure))
	record.GasRatioAvg, record.GasRatioMin, record.GasRatioMax = f32(pick(model.KindGasRatio))
	record.IlluminanceAvg, record.IlluminanceMin, record.IlluminanceMax = f32(pick(model.KindIlluminance))
	record.ColorTempAvg, record.ColorTempMin, record.ColorTempMax = f32(pick(model.KindColorTemp))

	record.DcVoltageAvg, record.DcVoltageMin, record.DcVoltageMax = f32(pick(model.KindDcVoltage))
	record.DcCurrentAvg, record.DcCurrentMin, record.DcCurrentMax = f32(pick(model.KindDcCurrent))
	record.DcPowerAvg, record.DcPowerMin, record.DcPowerMax = f32(pick(model.KindDcPower))

	record.ChipTemperatureAvg, _, _ = f32(pick(model.KindChipTemperature))
	return record
}

func toUint(v *float32) *uint {
	if v == nil {
		return nil
	}
	return new(uint(math.Round(float64(max(0, *v)))))
}
