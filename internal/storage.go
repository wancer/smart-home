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

type Storage struct {
	db                *gorm.DB
	buffer            []*model.SensorEventModel
	lock              sync.Mutex
	lastHistory       map[uint]string
	bufferFlushStream chan struct{}
	deviceMap         *DeviceMap
}

func NewStorage(db *gorm.DB, cfg *config.Config, deviceMap *DeviceMap) (*Storage, error) {
	s := &Storage{
		db:                db,
		buffer:            []*model.SensorEventModel{},
		deviceMap:         deviceMap,
		lastHistory:       map[uint]string{},
		bufferFlushStream: make(chan struct{}),
	}
	err := s.init(cfg)
	if err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Storage) init(c *config.Config) error {
	ctx := context.Background()
	// Load last recorded date per device from sensor_event
	for _, device := range s.deviceMap.GetAll() {
		lastEvents, err := gorm.G[model.SensorHistoryModel](s.db).
			Where("device_id = ?", device.ID).
			Order("date DESC").
			Limit(1).
			Find(ctx)
		if err != nil {
			return err
		}

		if len(lastEvents) == 0 {
			slog.Warn("No last record for", "topic", device.Topic)
			s.lastHistory[device.ID] = ""
		} else {
			lastEvent := lastEvents[0]
			s.lastHistory[device.ID] = time.Time(lastEvent.Date).Format(time.DateOnly)
			slog.Debug("Loaded last history", "topic", device.Topic, "device_id", device.ID, "last_date", s.lastHistory[device.ID])
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

func (s *Storage) Flush() {
	if len(s.buffer) == 0 {
		return
	}

	s.lock.Lock()
	buffer := s.buffer
	s.buffer = []*model.SensorEventModel{}
	s.lock.Unlock()

	for _, r := range buffer {
		if err := s.db.Save(&r).Error; err != nil {
			slog.Error(err.Error())
		}
		slog.Debug("stored sensor", "r", r)
	}

	slog.Info(fmt.Sprintf("Stored %d events", len(buffer)))
}

func (s *Storage) GetBuffer() []*model.SensorEventModel {
	s.lock.Lock()
	defer s.lock.Unlock()
	buffer := s.buffer

	return buffer
}

func (s *Storage) Store(m *model.SensorEventModel) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.buffer = append(s.buffer, m)
	slog.Debug("added to buffer", "event", m)
}

func (s *Storage) StoreDaily(e *event.SensorEvent, d *model.DeviceModel) {
	eventYesterday := time.Time(e.Time).AddDate(0, 0, -1)
	eventYesterdayStr := eventYesterday.Format(time.DateOnly)
	if eventYesterdayStr == s.lastHistory[d.ID] {
		return
	}

	history := model.SensorHistoryModel{}
	history.Date = datatypes.Date(eventYesterday)
	history.DeviceId = d.ID
	history.Power = e.Energy.Yesterday

	if err := s.db.Save(&history).Error; err != nil {
		slog.Error("CANT_SAVE_HISTORY", "err", err)
	} else {
		slog.Info("Save history", "date", eventYesterdayStr, "deviceId", d.ID)
		s.lastHistory[d.ID] = eventYesterdayStr
	}
}

func (s *Storage) Shutdown() {
	close(s.bufferFlushStream)
	s.Flush()
}
