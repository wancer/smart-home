package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"smart-home/internal"
	"smart-home/model"
	"strconv"
	"time"

	"gorm.io/gorm"
)

func NewSensorsController(db *gorm.DB, s *internal.Storage, states *internal.DeviceStateManager) *SensorsController {
	return &SensorsController{db: db, s: s, states: states}
}

type SensorsController struct {
	db     *gorm.DB
	s      *internal.Storage
	states *internal.DeviceStateManager
}

// GetMulti returns the recent values of the devices' series (dashboard sparklines),
// GET /sensors?ids=1&ids=2: device ID -> SeriesChart (step = the first storage tier's; stored
// buckets of the last 10 minutes or two first-tier steps, then each buffered reading).
func (c *SensorsController) GetMulti(w http.ResponseWriter, r *http.Request) {
	idParams := r.URL.Query()["ids"]
	if len(idParams) == 0 {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	var deviceIds []uint
	states := map[uint]*internal.DeviceState{}
	for _, param := range idParams {
		id, err := strconv.Atoi(param)
		if err != nil || id <= 0 {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		uid := uint(id)
		state := c.states.GetById(uid)
		if state == nil || states[uid] != nil {
			continue
		}
		deviceIds = append(deviceIds, uid)
		states[uid] = state
	}

	now := time.Now()
	result := map[uint]SeriesChart{}
	for _, id := range deviceIds {
		result[id] = SeriesChart{Series: map[uint][]SeriesPoint{}}
	}

	if len(deviceIds) > 0 {
		// The last 10 minutes of stored data; with coarse first-tier buckets, at least the
		// latest finished bucket of each device.
		window := 10 * time.Minute
		for _, state := range states {
			window = max(window, 2*state.Device.Tiers()[0].StepDuration())
		}
		since := now.Add(-window)
		stored, err := c.s.LoadRecentValues(r.Context(), deviceIds, since)
		if err != nil {
			slog.Error("[sensors][multi] error", "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		buffer := c.s.GetBuffer()

		for _, id := range deviceIds {
			chart := result[id]
			chart.Step = int64(states[id].Device.Tiers()[0].Step)
			chart.From, chart.To = since.Unix(), now.Unix()
			// stored buckets, then buffered (unflushed) readings one by one
			rows := stored[id]
			for _, v := range buffer {
				if v.DeviceId == id {
					rows = append(rows, model.SensorValue{SeriesId: v.SeriesId, BucketTime: v.At, Count: 1, Avg: v.Value, Min: v.Value, Max: v.Value, Sum: v.Sum})
				}
			}
			for _, point := range valuesByTime(rows) {
				for seriesId, v := range point.values {
					chart.Series[seriesId] = append(chart.Series[seriesId], newSeriesPoint(point.at, v))
				}
			}
			result[id] = chart
		}
	}

	slog.Info("[sensors][multi] success", "count", len(deviceIds))
	json.NewEncoder(w).Encode(result)
}

// timePoint is the values of a device's series at one time, keyed by series ID.
type timePoint struct {
	at     time.Time
	values map[uint]*model.SensorValue
}

// valuesByTime groups rows by bucket time (same instant, whatever the stored offset), in time
// order. Rows of one series at the same time are merged.
func valuesByTime(rows []model.SensorValue) []timePoint {
	byUnix := map[int64]*timePoint{}
	var points []*timePoint
	for i := range rows {
		row := rows[i]
		key := row.BucketTime.UnixNano()
		p, ok := byUnix[key]
		if !ok {
			p = &timePoint{at: row.BucketTime, values: map[uint]*model.SensorValue{}}
			byUnix[key] = p
			points = append(points, p)
		}
		if existing, ok := p.values[row.SeriesId]; ok {
			internal.MergeValue(existing, &row)
		} else {
			p.values[row.SeriesId] = &row
		}
	}
	slices.SortFunc(points, func(a, b *timePoint) int { return a.at.Compare(b.at) })
	result := make([]timePoint, len(points))
	for i, p := range points {
		result[i] = *p
	}
	return result
}
