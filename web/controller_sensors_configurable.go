package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"smart-home/internal"
	"smart-home/model"
	"strconv"
	"strings"
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

	var only []uint // nil = every series of the device (no ?series, "" or "all")
	if seriesParam, ok := r.URL.Query()["series"]; ok {
		if only, ok = parseSeriesIds(seriesParam); !ok {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
	}
	c.getSeries(w, r, state, only, duration, scaleParam == rawScale, scale, till)
}

// maxChartPoints caps duration/scale, e.g. 24h at a 15s step is 5760 points.
const maxChartPoints = 10_000

// rawScale returns every stored row as its own point (raw readings at their own time,
// coarser buckets at their start) instead of fixed steps. maxRawDuration caps its range.
const (
	rawScale       = "raw"
	maxRawDuration = 24 * time.Hour
)

// stepValues merges the rows of the device's series (only = nil: all) into clock-aligned
// steps: per step, series ID -> merged value (nil for an empty step). It returns the steps,
// the first step's start and the step actually used (raised to the coarsest stored bucket).
func (c *SensorsConfigurableController) stepValues(
	ctx context.Context,
	deviceId uint,
	only []uint,
	duration time.Duration,
	scale time.Duration,
	till time.Time,
) ([]map[uint]*model.SensorValue, time.Time, time.Duration, error) {
	from, end := chartRange(till, duration, scale)
	rows, err := c.buffer.LoadSeriesValues(ctx, deviceId, only, from, end)
	if err != nil {
		return nil, from, scale, err
	}

	if coarsest := coarsestBucket(rows); coarsest > scale {
		scale = coarsest
		wider, newEnd := chartRange(till, duration, scale)
		end = newEnd
		if wider.Before(from) {
			from = wider
			if rows, err = c.buffer.LoadSeriesValues(ctx, deviceId, only, from, end); err != nil {
				return nil, from, scale, err
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
	return points, from, scale, nil
}

// SeriesChart is a chart by series (GET /sensors/{duration}/{scale}?series=...): the points of
// each requested series that has data in [from, to). Step is the step used in seconds (raised
// to the coarsest stored bucket in range, like the fixed-field chart), 0 for raw.
type SeriesChart struct {
	Step   int64                  `json:"step"`
	From   int64                  `json:"from"`
	To     int64                  `json:"to"`
	Series map[uint][]SeriesPoint `json:"series"`
}

// SeriesPoint is one bucket of a series: time is the bucket (step) start; sum is the energy
// (Wh) of power series, null otherwise.
type SeriesPoint struct {
	Time  int64    `json:"time"`
	Avg   float64  `json:"avg"`
	Min   float64  `json:"min"`
	Max   float64  `json:"max"`
	Sum   *float64 `json:"sum,omitempty"`
	Count uint     `json:"count"`
}

func newSeriesPoint(at time.Time, v *model.SensorValue) SeriesPoint {
	return SeriesPoint{Time: at.Unix(), Avg: v.Avg, Min: v.Min, Max: v.Max, Sum: v.Sum, Count: v.Count}
}

// parseSeriesIds reads ?series=1,2 (or repeated ?series=1&series=2); "" or "all" = every
// series of the device (nil).
func parseSeriesIds(params []string) ([]uint, bool) {
	var ids []uint
	for _, param := range params {
		for part := range strings.SplitSeq(param, ",") {
			part = strings.TrimSpace(part)
			if part == "" || part == "all" {
				continue
			}
			id, err := strconv.ParseUint(part, 10, 32)
			if err != nil {
				return nil, false
			}
			ids = append(ids, uint(id))
		}
	}
	return ids, true
}

func (c *SensorsConfigurableController) getSeries(
	w http.ResponseWriter,
	r *http.Request,
	state *internal.DeviceState,
	only []uint,
	duration time.Duration,
	raw bool,
	scale time.Duration,
	till time.Time,
) {
	deviceId := state.Device.ID
	chart := SeriesChart{Series: map[uint][]SeriesPoint{}}
	if raw {
		if duration <= 0 || duration > maxRawDuration {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		from, end := till.Add(-duration), till.Add(time.Minute)
		rows, err := c.buffer.LoadSeriesValues(r.Context(), deviceId, only, from, end)
		if err != nil {
			slog.Error("[sensors][series] error", "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		chart.From, chart.To = from.Unix(), end.Unix()
		for _, p := range valuesByTime(rows) {
			for id, v := range p.values {
				chart.Series[id] = append(chart.Series[id], newSeriesPoint(p.at, v))
			}
		}
	} else {
		if scale <= 0 || duration <= 0 || duration/scale > maxChartPoints {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		points, from, step, err := c.stepValues(r.Context(), deviceId, only, duration, scale, till)
		if err != nil {
			slog.Error("[sensors][series] error", "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		chart.Step = int64(step / time.Second)
		chart.From, chart.To = from.Unix(), from.Add(time.Duration(len(points))*step).Unix()
		for i, point := range points {
			at := from.Add(time.Duration(i) * step)
			for _, id := range slices.Sorted(maps.Keys(point)) {
				chart.Series[id] = append(chart.Series[id], newSeriesPoint(at, point[id]))
			}
		}
	}
	json.NewEncoder(w).Encode(chart)
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
