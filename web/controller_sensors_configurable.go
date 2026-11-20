package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"slices"
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

// rawRecords returns the rows in (till-duration, till] by time. till is truncated to the
// minute, so the range runs a minute past it to include the latest readings.
func (c *SensorsConfigurableController) rawRecords(
	ctx context.Context,
	state *internal.DeviceState,
	duration time.Duration,
	till time.Time,
) ([]DeviceSensorEvent, error) {
	rows, err := c.loadAggregates(ctx, state.Device.ID, till.Add(-duration), till.Add(time.Minute))
	if err != nil {
		return nil, err
	}
	slices.SortFunc(rows, func(a, b model.SensorAggregate) int {
		return a.BucketTime.Compare(b.BucketTime)
	})

	records := make([]DeviceSensorEvent, 0, len(rows))
	for i := 0; i < len(rows); i++ {
		row := rows[i]
		// a buffered reading can share its second with a stored one
		for i+1 < len(rows) && rows[i+1].BucketTime.Equal(row.BucketTime) {
			i++
			internal.MergeInto(&row, &rows[i])
		}
		records = append(records, newDeviceSensorEvent(row.BucketTime, &row))
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
	from, end := chartRange(till, duration, scale)
	rows, err := c.loadAggregates(ctx, state.Device.ID, from, end)
	if err != nil {
		return nil, err
	}

	if coarsest := coarsestBucket(rows); coarsest > scale {
		scale = coarsest
		wider, newEnd := chartRange(till, duration, scale)
		end = newEnd
		if wider.Before(from) {
			from = wider
			if rows, err = c.loadAggregates(ctx, state.Device.ID, from, end); err != nil {
				return nil, err
			}
		}
	}

	points := make([]*model.SensorAggregate, int(end.Sub(from)/scale))
	for i := range rows {
		bt := rows[i].BucketTime.UTC()
		if bt.Before(from) || !bt.Before(end) {
			continue
		}
		idx := int(bt.Sub(from) / scale)
		if points[idx] == nil {
			p := rows[i]
			points[idx] = &p
		} else {
			internal.MergeInto(points[idx], &rows[i])
		}
	}

	records := make([]DeviceSensorEvent, len(points))
	for i, point := range points {
		at := from.Add(time.Duration(i) * scale)
		records[i] = newDeviceSensorEvent(at, point)
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

// loadAggregates returns the device's stored rows in [from, end) plus the unflushed buffer.
func (c *SensorsConfigurableController) loadAggregates(ctx context.Context, deviceId uint, from, end time.Time) ([]model.SensorAggregate, error) {
	rows, err := gorm.G[model.SensorAggregate](c.db).
		Where("device_id = ?", deviceId).
		Where("datetime(bucket_time) >= datetime(?)", from.UTC().Format(time.DateTime)).
		Where("datetime(bucket_time) < datetime(?)", end.UTC().Format(time.DateTime)).
		Order("bucket_time ASC").
		Find(ctx)
	if err != nil {
		return nil, err
	}
	return append(rows, c.buffer.GetBufferAggregates(deviceId)...), nil
}

func coarsestBucket(rows []model.SensorAggregate) time.Duration {
	var coarsest uint
	for _, row := range rows {
		coarsest = max(coarsest, row.BucketSeconds)
	}
	return time.Duration(coarsest) * time.Second
}

// newDeviceSensorEvent maps a merged step to a chart point; agg is nil for an empty step.
func newDeviceSensorEvent(at time.Time, agg *model.SensorAggregate) DeviceSensorEvent {
	record := DeviceSensorEvent{Time: at.Unix()}
	if agg == nil {
		return record
	}

	record.PowerConsumed = agg.PowerConsumed
	if record.PowerConsumed == nil && agg.PowerAvg != nil {
		record.PowerConsumed = new(float32(0))
	}
	record.PowerAvg = toUint(agg.PowerAvg)
	record.PowerMin = toUint(agg.PowerMin)
	record.PowerMax = toUint(agg.PowerMax)
	record.CurrentAvg = agg.CurrentAvg
	record.CurrentMin = agg.CurrentMin
	record.CurrentMax = agg.CurrentMax
	record.VoltageAvg = toUint(agg.VoltageAvg)
	record.VoltageMin = toUint(agg.VoltageMin)
	record.VoltageMax = toUint(agg.VoltageMax)

	record.CO2Avg = toUint(agg.CO2Avg)
	record.CO2Min = toUint(agg.CO2Min)
	record.CO2Max = toUint(agg.CO2Max)
	record.TemperatureAvg = agg.TemperatureAvg
	record.TemperatureMin = agg.TemperatureMin
	record.TemperatureMax = agg.TemperatureMax
	record.HumidityAvg = agg.HumidityAvg
	record.HumidityMin = agg.HumidityMin
	record.HumidityMax = agg.HumidityMax
	record.PressureAvg = agg.PressureAvg
	record.PressureMin = agg.PressureMin
	record.PressureMax = agg.PressureMax

	record.DcVoltageAvg = agg.DcVoltageAvg
	record.DcVoltageMin = agg.DcVoltageMin
	record.DcVoltageMax = agg.DcVoltageMax
	record.DcCurrentAvg = agg.DcCurrentAvg
	record.DcCurrentMin = agg.DcCurrentMin
	record.DcCurrentMax = agg.DcCurrentMax
	record.DcPowerAvg = agg.DcPowerAvg
	record.DcPowerMin = agg.DcPowerMin
	record.DcPowerMax = agg.DcPowerMax

	record.ChipTemperatureAvg = agg.ChipTemperatureAvg
	return record
}

func toUint(v *float32) *uint {
	if v == nil {
		return nil
	}
	return new(uint(math.Round(float64(max(0, *v)))))
}
