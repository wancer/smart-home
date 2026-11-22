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
)

// ChipTemperatureRetention is how long the ESP32 chip temperature is kept in sensor_aggregate.
const ChipTemperatureRetention = 24 * time.Hour

// maintenancePeriod is how often chip temperatures are pruned and storage tiers applied.
const maintenancePeriod = time.Hour

type Storage struct {
	db                *gorm.DB
	buffer            []*model.SensorEvent
	lock              *sync.Mutex
	lastHistory       map[uint]string
	bufferFlushStream chan struct{}
	deviceMap         *DeviceStateManager
	archivedLock      sync.Mutex
	archivedRows      map[uint]int64
	lastReading       map[uint]time.Time // last power reading per device, for its energy
}

// SensorHistorySize is the number of stored rows per sensor table for a device.
type SensorHistorySize struct {
	Aggregates int64  // sensor_aggregate rows, any bucket size
	Daily      int64  // sensor_history, one row per day
	Archived   int64  // sensor_event, legacy raw readings
	Oldest     *int64 // unix time of the oldest aggregate bucket, nil if there are none
}

func NewStorage(db *gorm.DB, cfg *config.Config, deviceStates *DeviceStateManager) (*Storage, error) {
	s := &Storage{
		db:                db,
		buffer:            []*model.SensorEvent{},
		deviceMap:         deviceStates,
		lastHistory:       map[uint]string{},
		lastReading:       map[uint]time.Time{},
		bufferFlushStream: make(chan struct{}),
		lock:              &sync.Mutex{},
	}
	err := s.init(cfg)
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

	s.PruneChipTemperature()

	ticker := time.NewTicker(c.Storage.FlushPeriod)
	maintenanceTicker := time.NewTicker(maintenancePeriod)
	go func() {
		// Rollups run in this goroutine so they never overlap a flush. The first one
		// can take a while after a device's tiers changed, so it doesn't block startup.
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
				return
			}
		}
	}()

	return nil
}

// PruneChipTemperature drops chip temperatures older than ChipTemperatureRetention.
// Rows left without any value (IR remotes only report the chip temperature) are deleted.
func (s *Storage) PruneChipTemperature() {
	cutoff := time.Now().Add(-ChipTemperatureRetention).UTC().Format(time.DateTime)
	old := s.db.Model(&model.SensorAggregate{}).
		Where("chip_temperature_avg IS NOT NULL").
		Where("datetime(bucket_time) < datetime(?)", cutoff)
	if err := old.Update("chip_temperature_avg", nil).Error; err != nil {
		slog.Error("prune chip temperature", "err", err)
		return
	}

	res := s.db.
		Where("datetime(bucket_time) < datetime(?)", cutoff).
		Where("chip_temperature_avg IS NULL AND power_consumed IS NULL AND power_avg IS NULL AND current_avg IS NULL AND voltage_avg IS NULL").
		Where("co2_avg IS NULL AND temperature_avg IS NULL AND humidity_avg IS NULL AND dew_point_avg IS NULL").
		Where("pressure_avg IS NULL AND dc_voltage_avg IS NULL AND dc_current_avg IS NULL AND dc_power_avg IS NULL AND gas_avg IS NULL").
		Delete(&model.SensorAggregate{})
	if res.Error != nil {
		slog.Error("prune empty aggregates", "err", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		slog.Info("Pruned chip-temperature-only aggregates", "rows", res.RowsAffected)
	}
}

func (s *Storage) Flush() {
	s.lock.Lock()
	buffer := s.buffer
	s.buffer = []*model.SensorEvent{}
	s.lock.Unlock()
	if len(buffer) == 0 {
		return
	}

	byDevice := map[uint][]*model.SensorEvent{}
	for _, r := range buffer {
		byDevice[r.DeviceId] = append(byDevice[r.DeviceId], r)
	}

	aggCount := 0
	for deviceId, events := range byDevice {
		for _, agg := range groupReadings(events, s.firstStep(deviceId)) {
			if err := upsertAggregate(s.db, agg); err != nil {
				slog.Error("store aggregate", "err", err)
			}
			aggCount++
		}
	}

	slog.Info(fmt.Sprintf("Stored %d aggregates from %d events", aggCount, len(buffer)))
}

// upsertAggregate saves agg, merging it into the row already stored for its bucket
// (e.g. after a restart mid-bucket, or a rollup into an existing coarser bucket).
func upsertAggregate(db *gorm.DB, agg *model.SensorAggregate) error {
	var existing []model.SensorAggregate
	err := db.Where("device_id = ? AND bucket_time = ?", agg.DeviceId, agg.BucketTime).Limit(1).Find(&existing).Error
	if err != nil {
		return err
	}
	if len(existing) == 0 {
		return db.Save(agg).Error
	}
	MergeInto(&existing[0], agg)
	return db.Save(&existing[0]).Error
}

// firstStep is the bucket size new readings of the device are stored in.
func (s *Storage) firstStep(deviceId uint) uint {
	if state := s.deviceMap.GetById(deviceId); state != nil {
		return state.Device.Tiers()[0].Step
	}
	return model.DefaultStorageTiers[0].Step
}

// GetBufferAggregates returns an on-the-fly aggregated view of buffered (unflushed) events
// for a single device, so controllers can include pending data alongside DB records.
func (s *Storage) GetBufferAggregates(deviceId uint) []model.SensorAggregate {
	s.lock.Lock()
	var events []*model.SensorEvent
	for _, e := range s.buffer {
		if e.DeviceId == deviceId {
			events = append(events, e)
		}
	}
	s.lock.Unlock()
	if len(events) == 0 {
		return nil
	}

	buckets := groupReadings(events, s.firstStep(deviceId))
	result := make([]model.SensorAggregate, 0, len(buckets))
	for _, agg := range buckets {
		result = append(result, *agg)
	}
	return result
}

func (s *Storage) GetBuffer() []*model.SensorEvent {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.buffer
}

func (s *Storage) Store(m *model.SensorEvent) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if m.Power != nil {
		hours := s.readingInterval(m.DeviceId, m.RealTime).Hours()
		m.EnergyWh = new(float32(float64(*m.Power) * hours))
	}
	s.buffer = append(s.buffer, m)
	slog.Debug("added to buffer", "event", m)
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

// GetHistorySizes returns stored row counts keyed by device ID. Devices with no
// stored rows are absent from the result.
func (s *Storage) GetHistorySizes(ctx context.Context) (map[uint]SensorHistorySize, error) {
	var aggregates []struct {
		DeviceId uint
		Total    int64
		Oldest   *int64
	}
	err := s.db.WithContext(ctx).Model(&model.SensorAggregate{}).
		Select("device_id, COUNT(*) AS total, CAST(strftime('%s', MIN(bucket_time)) AS INTEGER) AS oldest").
		Group("device_id").
		Scan(&aggregates).Error
	if err != nil {
		return nil, err
	}

	daily, err := countByDevice(s.db.WithContext(ctx).Model(&model.SensorHistory{}))
	if err != nil {
		return nil, err
	}

	archived, err := s.archivedCounts(ctx)
	if err != nil {
		return nil, err
	}

	sizes := map[uint]SensorHistorySize{}
	for _, a := range aggregates {
		sizes[a.DeviceId] = SensorHistorySize{Aggregates: a.Total, Oldest: a.Oldest}
	}
	for deviceId, total := range daily {
		size := sizes[deviceId]
		size.Daily = total
		sizes[deviceId] = size
	}
	for deviceId, total := range archived {
		size := sizes[deviceId]
		size.Archived = total
		sizes[deviceId] = size
	}
	return sizes, nil
}

// archivedCounts counts sensor_event rows once and caches the result: the table
// is no longer written to, and its index doesn't lead with device_id, so counting
// has to scan every row. Anything that deletes from it must reset archivedRows.
func (s *Storage) archivedCounts(ctx context.Context) (map[uint]int64, error) {
	s.archivedLock.Lock()
	defer s.archivedLock.Unlock()

	if s.archivedRows != nil {
		return s.archivedRows, nil
	}

	counts, err := countByDevice(s.db.WithContext(ctx).Model(&model.SensorEvent{}))
	if err != nil {
		return nil, err
	}
	s.archivedRows = counts
	return counts, nil
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

func (s *Storage) ClearBuffer(deviceId uint) {
	s.lock.Lock()
	defer s.lock.Unlock()
	filtered := s.buffer[:0]
	for _, e := range s.buffer {
		if e.DeviceId != deviceId {
			filtered = append(filtered, e)
		}
	}
	s.buffer = filtered
}

func (s *Storage) Shutdown() {
	close(s.bufferFlushStream)
	s.Flush()
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
