package internal

import (
	"context"
	"fmt"
	"log/slog"
	"smart-home/config"
	"smart-home/event"
	"smart-home/model"
	"sync"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChipTemperatureRetention is how long chip_temperature series (ESP32) are kept.
const ChipTemperatureRetention = 24 * time.Hour

// maintenancePeriod is how often chip temperatures are pruned and storage tiers applied.
const maintenancePeriod = time.Hour

// upsertBatch is how many sensor_value rows one INSERT statement writes.
const upsertBatch = 500

// A failed flush keeps its readings for the next one, within limits: the buffer never holds
// more than maxBufferedReadings (the oldest beyond are dropped), and readings that failed
// maxFlushFailures flushes in a row are dropped, so one bad write can't block the rest forever.
const (
	maxBufferedReadings = 200_000
	maxFlushFailures    = 60
)

type Storage struct {
	db                *gorm.DB
	series            *seriesCache
	buffer            []BufferedValue
	lock              *sync.Mutex // guards buffer and lastReading
	// writeLock serialises the writers of sensor_value: a flush, a series' rollup/retention, the
	// chip temperature prune and a device's series deletion (DeleteDevice), so nothing is written
	// into a series while it is deleted.
	writeLock sync.Mutex
	done      chan struct{} // closed by the storage goroutine after its final flush
	// flushFailures counts flushes failed in a row (guarded by writeLock)
	flushFailures int
	lastHistory       map[uint]string
	bufferFlushStream chan struct{}
	deviceMap         *DeviceStateManager
	lastReading       map[uint]time.Time // last power reading per device, for its energy
}

// SensorHistorySize is the number of stored rows per sensor table for a device.
type SensorHistorySize struct {
	Aggregates int64  // sensor_value rows of all the device's series, any bucket size
	Daily      int64  // sensor_history, one row per day
	Oldest     *int64 // unix time of the oldest bucket, nil if there are none
}

func NewStorage(db *gorm.DB, cfg *config.Config, deviceStates *DeviceStateManager) (*Storage, error) {
	series, err := newSeriesCache(db)
	if err != nil {
		return nil, err
	}
	s := &Storage{
		db:                db,
		series:            series,
		deviceMap:         deviceStates,
		lastHistory:       map[uint]string{},
		lastReading:       map[uint]time.Time{},
		bufferFlushStream: make(chan struct{}),
		done:              make(chan struct{}),
		lock:              &sync.Mutex{},
	}
	err = s.init(cfg)
	if err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Storage) init(c *config.Config) error {
	ctx := context.Background()
	for _, device := range s.deviceMap.GetAll() {
		lastEvents, err := gorm.G[model.SensorHistory](s.db).
			Where("device_id = ?", device.Device.ID).
			Order("date DESC").
			Limit(1).
			Find(ctx)
		if err != nil {
			return err
		}

		if len(lastEvents) == 0 {
			slog.Warn("No last record for", "topic", device.Device.Topic)
			s.lastHistory[device.Device.ID] = ""
		} else {
			lastEvent := lastEvents[0]
			s.lastHistory[device.Device.ID] = time.Time(lastEvent.Date).Format(time.DateOnly)
			slog.Debug("Loaded last history", "topic", device.Device.Topic, "device_id", device.Device.ID, "last_date", s.lastHistory[device.Device.ID])
		}
	}

	ticker := time.NewTicker(c.Storage.FlushPeriod)
	maintenanceTicker := time.NewTicker(maintenancePeriod)
	go func() {
		// Rollups run in this goroutine so they never overlap a flush. The first one
		// can take a while after a device's tiers changed, so it doesn't block startup.
		s.PruneChipTemperature()
		s.ApplyStorageTiers()
		for {
			select {
			case <-ticker.C:
				s.Flush()
			case <-maintenanceTicker.C:
				s.PruneChipTemperature()
				s.ApplyStorageTiers()
			case <-s.bufferFlushStream:
				ticker.Stop()
				maintenanceTicker.Stop()
				s.Flush() // the final flush, in this goroutine so it never overlaps a rollup
				close(s.done)
				return
			}
		}
	}()

	return nil
}

// PruneChipTemperature deletes the values of chip_temperature series (and only those)
// older than ChipTemperatureRetention.
func (s *Storage) PruneChipTemperature() {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()
	cutoff := time.Now().Add(-ChipTemperatureRetention).UTC().Format(time.DateTime)
	res := s.db.
		Where("series_id IN (?)", s.db.Model(&model.SensorSeries{}).Select("id").Where("kind = ?", model.KindChipTemperature)).
		Where("datetime(bucket_time) < datetime(?)", cutoff).
		Delete(&model.SensorValue{})
	if res.Error != nil {
		slog.Error("prune chip temperature", "err", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		slog.Info("Pruned chip temperatures", "rows", res.RowsAffected)
	}
}

func (s *Storage) Flush() {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()

	s.lock.Lock()
	taken := s.buffer
	s.buffer = nil
	s.lock.Unlock()
	// readings of series deleted meanwhile (device delete / wipe) are dropped
	buffer := s.liveReadings(taken)
	if len(buffer) == 0 {
		return
	}

	byDevice := map[uint][]BufferedValue{}
	lastSeen := map[uint]time.Time{}
	for _, v := range buffer {
		byDevice[v.DeviceId] = append(byDevice[v.DeviceId], v)
		if at := v.At.UTC(); at.After(lastSeen[v.SeriesId]) {
			lastSeen[v.SeriesId] = at
		}
	}
	var values []*model.SensorValue
	for deviceId, readings := range byDevice {
		values = append(values, groupValues(readings, s.firstStep(deviceId))...)
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := upsertValues(tx, values); err != nil {
			return err
		}
		for id, at := range lastSeen {
			err := tx.Model(&model.SensorSeries{}).Where("id = ?", id).Update("last_seen", at).Error
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		s.requeue(buffer, err)
		return
	}
	s.flushFailures = 0
	s.series.touch(lastSeen)

	slog.Info(fmt.Sprintf("Stored %d sensor values from %d readings", len(values), len(buffer)))
}

// requeue puts the readings of a failed flush (nothing was written: one transaction) back in
// front of the buffer for the next one, within maxFlushFailures / maxBufferedReadings.
// Must be called with writeLock held.
func (s *Storage) requeue(failed []BufferedValue, err error) {
	s.flushFailures++
	if s.flushFailures >= maxFlushFailures {
		slog.Error("STORING SENSOR VALUES FAILED repeatedly, dropping the oldest readings",
			"err", err, "failures", s.flushFailures, "dropped", len(failed))
		s.flushFailures = 0
		return
	}

	s.lock.Lock()
	s.buffer = append(failed, s.buffer...)
	dropped := 0
	if over := len(s.buffer) - maxBufferedReadings; over > 0 {
		dropped = over
		s.buffer = append([]BufferedValue(nil), s.buffer[over:]...)
	}
	pending := len(s.buffer)
	s.lock.Unlock()

	slog.Error("store sensor values, retrying on the next flush",
		"err", err, "failures", s.flushFailures, "readings", len(failed), "buffered", pending)
	if dropped > 0 {
		slog.Error("sensor buffer full, dropped the oldest readings", "dropped", dropped, "cap", maxBufferedReadings)
	}
}

// liveReadings returns the readings whose series still exist (are cached).
func (s *Storage) liveReadings(readings []BufferedValue) []BufferedValue {
	live := readings[:0:0]
	for _, v := range readings {
		if s.series.has(v.SeriesId) {
			live = append(live, v)
		}
	}
	return live
}

// upsertValues saves the buckets, merging each into the row already stored for its series
// and bucket time (e.g. after a restart mid-bucket, or a rollup into an existing coarser
// bucket) the same way MergeValue does. All expressions read the old row.
func upsertValues(db *gorm.DB, values []*model.SensorValue) error {
	if len(values) == 0 {
		return nil
	}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "series_id"}, {Name: "bucket_time"}},
		DoUpdates: clause.Assignments(map[string]any{
			"bucket_seconds": gorm.Expr(`max("sensor_value"."bucket_seconds", excluded."bucket_seconds")`),
			"count":          gorm.Expr(`"sensor_value"."count" + excluded."count"`),
			"avg": gorm.Expr(`CASE WHEN "sensor_value"."count" + excluded."count" = 0 THEN "sensor_value"."avg" ` +
				`ELSE ("sensor_value"."avg" * "sensor_value"."count" + excluded."avg" * excluded."count") / ("sensor_value"."count" + excluded."count") END`),
			"min": gorm.Expr(`min("sensor_value"."min", excluded."min")`),
			"max": gorm.Expr(`max("sensor_value"."max", excluded."max")`),
			"sum": gorm.Expr(`CASE WHEN "sensor_value"."sum" IS NULL THEN excluded."sum" ` +
				`WHEN excluded."sum" IS NULL THEN "sensor_value"."sum" ELSE "sensor_value"."sum" + excluded."sum" END`),
		}),
	}).CreateInBatches(values, upsertBatch).Error
}

// firstStep is the bucket size new readings of the device are stored in.
func (s *Storage) firstStep(deviceId uint) uint {
	if state := s.deviceMap.GetById(deviceId); state != nil {
		return state.Device.Tiers()[0].Step
	}
	return model.DefaultStorageTiers[0].Step
}

// GetBufferAggregates returns an on-the-fly bucketed view of the device's buffered (unflushed)
// readings, one row per series and bucket, so controllers can include pending data alongside
// DB records.
func (s *Storage) GetBufferAggregates(deviceId uint) []model.SensorValue {
	s.lock.Lock()
	var readings []BufferedValue
	for _, v := range s.buffer {
		if v.DeviceId == deviceId {
			readings = append(readings, v)
		}
	}
	s.lock.Unlock()
	if len(readings) == 0 {
		return nil
	}

	buckets := groupValues(readings, s.firstStep(deviceId))
	result := make([]model.SensorValue, 0, len(buckets))
	for _, b := range buckets {
		result = append(result, *b)
	}
	return result
}

// GetBuffer returns a copy of the buffered (unflushed) readings.
func (s *Storage) GetBuffer() []BufferedValue {
	s.lock.Lock()
	defer s.lock.Unlock()
	return append([]BufferedValue(nil), s.buffer...)
}

// Store buffers the readings of one message, received at `at`. Series are created on first
// sight. A power reading's Sum is the energy it stands for (Wh = power × readingInterval).
// It returns the values keyed by series ID (the device's live values).
func (s *Storage) Store(deviceId uint, at time.Time, readings []event.Reading) map[uint]float64 {
	values, kinds := s.resolveReadings(deviceId, readings, at)
	if len(values) == 0 {
		return values
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	var hours *float64
	for id, value := range values {
		v := BufferedValue{SeriesId: id, DeviceId: deviceId, At: at.UTC(), Value: value}
		if kinds[id] == model.KindPower {
			if hours == nil {
				hours = new(s.readingInterval(deviceId, at).Hours())
			}
			v.Sum = new(value * *hours)
		}
		s.buffer = append(s.buffer, v)
	}
	slog.Debug("added to buffer", "device_id", deviceId, "values", len(values))
	return values
}

// resolveReadings maps readings to their series (created on first sight): values and kinds by
// series ID. A reading whose series can't be created is logged and left out.
func (s *Storage) resolveReadings(deviceId uint, readings []event.Reading, at time.Time) (map[uint]float64, map[uint]string) {
	values := make(map[uint]float64, len(readings))
	kinds := make(map[uint]string, len(readings))
	for _, r := range readings {
		series, err := s.series.resolve(deviceId, r.Source, r.Field, at)
		if err != nil {
			slog.Error("create sensor series", "device_id", deviceId, "source", r.Source, "field", r.Field, "err", err)
			continue
		}
		values[series.Id] = r.Value
		kinds[series.Id] = series.Kind
	}
	return values, kinds
}

func (s *Storage) StoreDaily(e *event.SensorEvent, deviceId uint) {
	eventYesterday := time.Time(e.Time).AddDate(0, 0, -1)
	eventYesterdayStr := eventYesterday.Format(time.DateOnly)
	if eventYesterdayStr == s.lastHistory[deviceId] {
		return
	}

	history := model.SensorHistory{}
	history.Date = datatypes.Date(eventYesterday)
	history.DeviceId = deviceId
	history.Power = e.Energy.Yesterday

	if err := s.db.Save(&history).Error; err != nil {
		slog.Error("CANT_SAVE_HISTORY", "err", err)
	} else {
		slog.Info("Save history", "date", eventYesterdayStr, "deviceId", deviceId)
		s.lastHistory[deviceId] = eventYesterdayStr
	}
}

// LoadValues returns the stored values of the device's series with bucket start in
// [from, end), plus the unflushed buffer.
func (s *Storage) LoadValues(ctx context.Context, deviceId uint, from, end time.Time) ([]model.SensorValue, error) {
	ids := s.series.deviceIds(deviceId)
	var rows []model.SensorValue
	if len(ids) > 0 {
		err := s.db.WithContext(ctx).
			Where("series_id IN ?", ids).
			Where("datetime(bucket_time) >= datetime(?)", from.UTC().Format(time.DateTime)).
			Where("datetime(bucket_time) < datetime(?)", end.UTC().Format(time.DateTime)).
			Order("bucket_time ASC").
			Find(&rows).Error
		if err != nil {
			return nil, err
		}
	}
	return append(rows, s.GetBufferAggregates(deviceId)...), nil
}

// LoadRecentValues returns the stored values of the devices' series with bucket start after
// `since`, keyed by device ID (not including the buffer).
func (s *Storage) LoadRecentValues(ctx context.Context, deviceIds []uint, since time.Time) (map[uint][]model.SensorValue, error) {
	deviceOf := map[uint]uint{}
	var ids []uint
	for _, deviceId := range deviceIds {
		for _, id := range s.series.deviceIds(deviceId) {
			deviceOf[id] = deviceId
			ids = append(ids, id)
		}
	}
	result := map[uint][]model.SensorValue{}
	if len(ids) == 0 {
		return result, nil
	}
	var rows []model.SensorValue
	err := s.db.WithContext(ctx).
		Where("series_id IN ?", ids).
		Where("datetime(bucket_time) > datetime(?)", since.UTC().Format(time.DateTime)).
		Order("bucket_time ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[deviceOf[row.SeriesId]] = append(result[deviceOf[row.SeriesId]], row)
	}
	return result, nil
}

// GetHistorySizes returns stored row counts keyed by device ID. Devices with no
// stored rows are absent from the result.
func (s *Storage) GetHistorySizes(ctx context.Context) (map[uint]SensorHistorySize, error) {
	var values []struct {
		DeviceId uint
		Total    int64
		Oldest   *int64
	}
	err := s.db.WithContext(ctx).Table("sensor_value AS v").
		Joins("JOIN sensor_series AS s ON s.id = v.series_id").
		Select("s.device_id AS device_id, COUNT(*) AS total, CAST(strftime('%s', MIN(v.bucket_time)) AS INTEGER) AS oldest").
		Group("s.device_id").
		Scan(&values).Error
	if err != nil {
		return nil, err
	}

	daily, err := countByDevice(s.db.WithContext(ctx).Model(&model.SensorHistory{}))
	if err != nil {
		return nil, err
	}

	sizes := map[uint]SensorHistorySize{}
	for _, a := range values {
		sizes[a.DeviceId] = SensorHistorySize{Aggregates: a.Total, Oldest: a.Oldest}
	}
	for deviceId, total := range daily {
		size := sizes[deviceId]
		size.Daily = total
		sizes[deviceId] = size
	}
	return sizes, nil
}

func countByDevice(query *gorm.DB) (map[uint]int64, error) {
	var rows []struct {
		DeviceId uint
		Total    int64
	}
	if err := query.Select("device_id, COUNT(*) AS total").Group("device_id").Scan(&rows).Error; err != nil {
		return nil, err
	}

	counts := make(map[uint]int64, len(rows))
	for _, row := range rows {
		counts[row.DeviceId] = row.Total
	}
	return counts, nil
}

// DeleteDeviceSeries deletes the device's series and all their values (tx may be a
// transaction). Use Storage.DeleteDevice, which keeps flushes and rollups out meanwhile.
func DeleteDeviceSeries(tx *gorm.DB, deviceId uint) error {
	seriesIds := tx.Model(&model.SensorSeries{}).Select("id").Where("device_id = ?", deviceId)
	if err := tx.Where("series_id IN (?)", seriesIds).Delete(&model.SensorValue{}).Error; err != nil {
		return err
	}
	return tx.Where("device_id = ?", deviceId).Delete(&model.SensorSeries{}).Error
}

// DeleteDevice deletes the device's series and their values, plus whatever `also` deletes, in
// one transaction (device delete, sensor data wipe). It holds the write lock, so no flush or
// rollup writes into the series meanwhile, and drops the device's buffered readings and cached
// series before and after (new readings create new series).
func (s *Storage) DeleteDevice(db *gorm.DB, deviceId uint, also func(tx *gorm.DB) error) error {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()

	s.ForgetDevice(deviceId)
	err := db.Transaction(func(tx *gorm.DB) error {
		if also != nil {
			if err := also(tx); err != nil {
				return err
			}
		}
		return DeleteDeviceSeries(tx, deviceId)
	})
	s.ForgetDevice(deviceId)
	return err
}

// ForgetDevice drops the device's buffered readings and cached series.
func (s *Storage) ForgetDevice(deviceId uint) {
	s.ClearBuffer(deviceId)
	s.series.forgetDevice(deviceId)
}

func (s *Storage) ClearBuffer(deviceId uint) {
	s.lock.Lock()
	defer s.lock.Unlock()
	filtered := s.buffer[:0]
	for _, v := range s.buffer {
		if v.DeviceId != deviceId {
			filtered = append(filtered, v)
		}
	}
	s.buffer = filtered
}

// Shutdown stops the storage goroutine and waits for its final flush.
func (s *Storage) Shutdown() {
	close(s.bufferFlushStream)
	<-s.done
}

// readingInterval is how long a power reading stands for: the time since the device's
// previous reading, at most its TelePeriod (so a reading after an outage doesn't count
// the whole gap). The first reading counts a TelePeriod, or nothing if it isn't known yet.
// Must be called with s.lock held.
func (s *Storage) readingInterval(deviceId uint, at time.Time) time.Duration {
	limit := defaultTelePeriod
	known := false
	if state := s.deviceMap.GetById(deviceId); state != nil && state.Config.TelePeriod != nil && *state.Config.TelePeriod > 0 {
		limit = time.Duration(*state.Config.TelePeriod) * time.Second
		known = true
	}

	last, ok := s.lastReading[deviceId]
	s.lastReading[deviceId] = at
	if !ok {
		if known {
			return limit
		}
		return 0
	}
	return max(0, min(at.Sub(last), limit))
}
