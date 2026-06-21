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

const sensorFreqSeconds float32 = 15

type Storage struct {
	db                *gorm.DB
	buffer            []*model.SensorEvent
	lock              *sync.Mutex
	lastHistory       map[uint]string
	bufferFlushStream chan struct{}
	deviceMap         *DeviceStateManager
}

func NewStorage(db *gorm.DB, cfg *config.Config, deviceStates *DeviceStateManager) (*Storage, error) {
	s := &Storage{
		db:                db,
		buffer:            []*model.SensorEvent{},
		deviceMap:         deviceStates,
		lastHistory:       map[uint]string{},
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

	ticker := time.NewTicker(c.Storage.FlushPeriod)
	go func() {
		for {
			select {
			case <-ticker.C:
				s.Flush()
			case <-s.bufferFlushStream:
				ticker.Stop()
				return
			}
		}
	}()

	return nil
}

type aggregateBucket struct {
	count         uint
	powerConsumed float32
	powerSum      float32
	currentSum    float32
	voltageSum    float32
	hasEnergy     bool
	co2Sum        float32
	co2eSum       float32
	hasCo2        bool
	tempSum       float32
	humiditySum   float32
	dewPointSum   float32
	hasTH         bool
}

func groupIntoBuckets(events []*model.SensorEvent) map[time.Time]*aggregateBucket {
	buckets := map[time.Time]*aggregateBucket{}
	for _, r := range events {
		key := r.RealTime.UTC().Truncate(5 * time.Minute)
		d := buckets[key]
		if d == nil {
			d = &aggregateBucket{}
			buckets[key] = d
		}
		d.count++
		if r.Power != nil {
			d.powerConsumed += float32(*r.Power) * sensorFreqSeconds / 3600.0
			d.powerSum += float32(*r.Power)
			d.hasEnergy = true
		}
		if r.Current != nil {
			d.currentSum += *r.Current
		}
		if r.Voltage != nil {
			d.voltageSum += float32(*r.Voltage)
		}
		if r.CO2 != nil {
			d.co2Sum += float32(*r.CO2)
			d.hasCo2 = true
		}
		if r.CO2e != nil {
			d.co2eSum += float32(*r.CO2e)
		}
		if r.Temperature != nil {
			d.tempSum += *r.Temperature
			d.hasTH = true
		}
		if r.Humidity != nil {
			d.humiditySum += *r.Humidity
		}
		if r.DewPoint != nil {
			d.dewPointSum += *r.DewPoint
		}
	}
	return buckets
}

func bucketToAggregate(deviceId uint, bucketTime time.Time, data *aggregateBucket) model.SensorAggregate {
	n := float32(data.count)
	agg := model.SensorAggregate{
		DeviceId:   deviceId,
		BucketTime: bucketTime,
		Count:      data.count,
	}
	if data.hasEnergy {
		pc := data.powerConsumed
		pa := data.powerSum / n
		ca := data.currentSum / n
		va := data.voltageSum / n
		agg.PowerConsumed = &pc
		agg.PowerAvg = &pa
		agg.CurrentAvg = &ca
		agg.VoltageAvg = &va
	}
	if data.hasCo2 {
		co2 := data.co2Sum / n
		co2e := data.co2eSum / n
		agg.CO2Avg = &co2
		agg.CO2eAvg = &co2e
	}
	if data.hasTH {
		temp := data.tempSum / n
		hum := data.humiditySum / n
		dew := data.dewPointSum / n
		agg.TemperatureAvg = &temp
		agg.HumidityAvg = &hum
		agg.DewPointAvg = &dew
	}
	return agg
}

func mergeFloat32Sum(a, b *float32) *float32 {
	if a == nil && b == nil {
		return nil
	}
	var av, bv float32
	if a != nil {
		av = *a
	}
	if b != nil {
		bv = *b
	}
	v := av + bv
	return &v
}

func mergeWeightedAvg(a *float32, na float32, b *float32, nb float32, total float32) *float32 {
	if a == nil && b == nil {
		return nil
	}
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	v := (*a*na + *b*nb) / total
	return &v
}

// mergeAggregate combines new into existing using count-weighted averages.
// existing.ID is preserved so db.Save performs an UPDATE.
func mergeAggregate(existing, new *model.SensorAggregate) model.SensorAggregate {
	n1 := float32(existing.Count)
	n2 := float32(new.Count)
	total := n1 + n2
	return model.SensorAggregate{
		ID:             existing.ID,
		DeviceId:       existing.DeviceId,
		BucketTime:     existing.BucketTime,
		Count:          existing.Count + new.Count,
		PowerConsumed:  mergeFloat32Sum(existing.PowerConsumed, new.PowerConsumed),
		PowerAvg:       mergeWeightedAvg(existing.PowerAvg, n1, new.PowerAvg, n2, total),
		CurrentAvg:     mergeWeightedAvg(existing.CurrentAvg, n1, new.CurrentAvg, n2, total),
		VoltageAvg:     mergeWeightedAvg(existing.VoltageAvg, n1, new.VoltageAvg, n2, total),
		CO2Avg:         mergeWeightedAvg(existing.CO2Avg, n1, new.CO2Avg, n2, total),
		CO2eAvg:        mergeWeightedAvg(existing.CO2eAvg, n1, new.CO2eAvg, n2, total),
		TemperatureAvg: mergeWeightedAvg(existing.TemperatureAvg, n1, new.TemperatureAvg, n2, total),
		HumidityAvg:    mergeWeightedAvg(existing.HumidityAvg, n1, new.HumidityAvg, n2, total),
		DewPointAvg:    mergeWeightedAvg(existing.DewPointAvg, n1, new.DewPointAvg, n2, total),
	}
}

func (s *Storage) Flush() {
	if len(s.buffer) == 0 {
		return
	}

	s.lock.Lock()
	buffer := s.buffer
	s.buffer = []*model.SensorEvent{}
	s.lock.Unlock()

	byDevice := map[uint][]*model.SensorEvent{}
	for _, r := range buffer {
		byDevice[r.DeviceId] = append(byDevice[r.DeviceId], r)
	}

	aggCount := 0
	for deviceId, events := range byDevice {
		buckets := groupIntoBuckets(events)
		for bucketTime, data := range buckets {
			agg := bucketToAggregate(deviceId, bucketTime, data)

			var existing model.SensorAggregate
			err := s.db.Where("device_id = ? AND bucket_time = ?", deviceId, bucketTime).First(&existing).Error
			if err == nil {
				agg = mergeAggregate(&existing, &agg)
			}

			if err := s.db.Save(&agg).Error; err != nil {
				slog.Error("store aggregate", "err", err)
			}
			aggCount++
		}
	}

	slog.Info(fmt.Sprintf("Stored %d aggregates from %d events", aggCount, len(buffer)))
}

// GetBufferAggregates returns an on-the-fly aggregated view of buffered (unflushed) events
// for a single device, so controllers can include pending data alongside DB records.
func (s *Storage) GetBufferAggregates(deviceId uint) []model.SensorAggregate {
	s.lock.Lock()
	defer s.lock.Unlock()

	var events []*model.SensorEvent
	for _, e := range s.buffer {
		if e.DeviceId == deviceId {
			events = append(events, e)
		}
	}
	if len(events) == 0 {
		return nil
	}

	buckets := groupIntoBuckets(events)
	result := make([]model.SensorAggregate, 0, len(buckets))
	for bucketTime, data := range buckets {
		result = append(result, bucketToAggregate(deviceId, bucketTime, data))
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
