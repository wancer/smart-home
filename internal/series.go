package internal

import (
	"cmp"
	"context"
	"slices"
	"smart-home/event"
	"smart-home/model"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type seriesKey struct {
	deviceId      uint
	source, field string
}

// seriesCache maps (device, source, field) to its sensor_series row. Rows are created on
// first sight; the cache is loaded once at start and kept in sync by this process only.
type seriesCache struct {
	db    *gorm.DB
	lock  sync.Mutex
	byKey map[seriesKey]*model.SensorSeries
	byId  map[uint]*model.SensorSeries
}

func newSeriesCache(db *gorm.DB) (*seriesCache, error) {
	c := &seriesCache{db: db, byKey: map[seriesKey]*model.SensorSeries{}, byId: map[uint]*model.SensorSeries{}}
	var all []model.SensorSeries
	if err := db.Find(&all).Error; err != nil {
		return nil, err
	}
	for i := range all {
		c.add(&all[i])
	}
	return c, nil
}

// add must be called with c.lock held (or before the cache is shared).
func (c *seriesCache) add(s *model.SensorSeries) {
	c.byKey[seriesKey{s.DeviceId, s.Source, s.Field}] = s
	c.byId[s.Id] = s
}

// resolve returns the series of a reading, creating the row on first sight (created = it
// wasn't cached).
func (c *seriesCache) resolve(deviceId uint, source, field string, now time.Time) (s *model.SensorSeries, created bool, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	key := seriesKey{deviceId, source, field}
	if s, ok := c.byKey[key]; ok {
		return s, false, nil
	}

	now = now.UTC()
	row := model.SensorSeries{
		DeviceId: deviceId, Source: source, Field: field, Kind: model.SeriesKind(source, field),
		FirstSeen: now, LastSeen: now,
	}
	// DO NOTHING if it exists already; the ID is always read back, never trusted from the insert.
	if err := c.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return nil, false, err
	}
	var stored model.SensorSeries
	err = c.db.Where("device_id = ? AND source = ? AND field = ?", deviceId, source, field).First(&stored).Error
	if err != nil {
		return nil, false, err
	}
	c.add(&stored)
	return &stored, true, nil
}

// get returns a copy of the series, false if it isn't known.
func (c *seriesCache) get(id uint) (model.SensorSeries, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if s, ok := c.byId[id]; ok {
		return *s, true
	}
	return model.SensorSeries{}, false
}

// update sets the user-editable fields of a cached series.
func (c *seriesCache) update(id uint, label *string, hidden bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if s, ok := c.byId[id]; ok {
		s.Label, s.Hidden = label, hidden
	}
}

// device returns copies of the device's series, sorted by source and field.
func (c *seriesCache) device(deviceId uint) []model.SensorSeries {
	c.lock.Lock()
	defer c.lock.Unlock()
	var result []model.SensorSeries
	for _, s := range c.byId {
		if s.DeviceId == deviceId {
			result = append(result, *s)
		}
	}
	slices.SortFunc(result, func(a, b model.SensorSeries) int {
		return cmp.Or(cmp.Compare(a.Source, b.Source), cmp.Compare(a.Field, b.Field))
	})
	return result
}

func (c *seriesCache) deviceIds(deviceId uint) []uint {
	c.lock.Lock()
	defer c.lock.Unlock()
	var ids []uint
	for id, s := range c.byId {
		if s.DeviceId == deviceId {
			ids = append(ids, id)
		}
	}
	return ids
}

func (c *seriesCache) has(id uint) bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	_, ok := c.byId[id]
	return ok
}

func (c *seriesCache) touch(lastSeen map[uint]time.Time) {
	c.lock.Lock()
	defer c.lock.Unlock()
	for id, at := range lastSeen {
		if s, ok := c.byId[id]; ok && at.After(s.LastSeen) {
			s.LastSeen = at
		}
	}
}

func (c *seriesCache) forgetDevice(deviceId uint) {
	c.lock.Lock()
	defer c.lock.Unlock()
	for id, s := range c.byId {
		if s.DeviceId == deviceId {
			delete(c.byId, id)
			delete(c.byKey, seriesKey{s.DeviceId, s.Source, s.Field})
		}
	}
}

// SeriesValues resolves (creating on first sight) the series of the readings and returns
// their values keyed by series ID, without storing anything. Readings whose series can't be
// created are left out.
func (s *Storage) SeriesValues(deviceId uint, readings []event.Reading) map[uint]float64 {
	values, _ := s.resolveReadings(deviceId, readings, time.Now())
	return values
}

// DeviceSeries returns the device's series (metadata), sorted by source and field.
func (s *Storage) DeviceSeries(deviceId uint) []model.SensorSeries {
	return s.series.device(deviceId)
}

// Series returns one series (metadata), false if it doesn't exist.
func (s *Storage) Series(id uint) (model.SensorSeries, bool) {
	return s.series.get(id)
}

// UpdateSeries sets a series' user-set label (nil = derived from kind/source) and hidden flag.
func (s *Storage) UpdateSeries(ctx context.Context, id uint, label *string, hidden bool) error {
	err := s.db.WithContext(ctx).Model(&model.SensorSeries{}).Where("id = ?", id).
		Updates(map[string]any{"label": label, "hidden": hidden}).Error
	if err != nil {
		return err
	}
	s.series.update(id, label, hidden)
	if series, ok := s.series.get(id); ok && s.OnSeriesChange != nil {
		s.OnSeriesChange(series.DeviceId)
	}
	return nil
}

// PreferredSeriesOf returns one device's series IDs per kind, the preferred one first: a
// dedicated temperature/humidity sensor before a CO₂ sensor's, a light meter's illuminance before
// a colour sensor's, and migrated (SourceLegacy) data last. Used by the startup migration of
// metric-key dashboard views and notification rules.
func PreferredSeriesOf(series []model.SensorSeries) map[string][]uint {
	hasKind := map[string]bool{}  // source + "\x00" + kind
	hasField := map[string]bool{} // source + "\x00" + field
	for _, s := range series {
		hasKind[s.Source+"\x00"+s.Kind] = true
		hasField[s.Source+"\x00"+s.Field] = true
	}
	rank := func(s model.SensorSeries) int {
		switch {
		case s.Source == model.SourceLegacy:
			return 2
		case (s.Kind == model.KindTemperature || s.Kind == model.KindHumidity || s.Kind == model.KindDewPoint) &&
			hasKind[s.Source+"\x00"+model.KindCo2]:
			return 1
		case s.Kind == model.KindIlluminance && hasField[s.Source+"\x00Red"]:
			return 1
		}
		return 0
	}

	sorted := slices.Clone(series)
	slices.SortStableFunc(sorted, func(a, b model.SensorSeries) int {
		return cmp.Or(cmp.Compare(rank(a), rank(b)), cmp.Compare(a.Source, b.Source),
			cmp.Compare(a.Field, b.Field), cmp.Compare(a.Id, b.Id))
	})
	result := map[string][]uint{}
	for _, s := range sorted {
		result[s.Kind] = append(result[s.Kind], s.Id)
	}
	return result
}
