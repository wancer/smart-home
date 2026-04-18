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
	lock              *sync.Mutex
	lastHistory       map[uint]string
	bufferFlushStream chan struct{}
	deviceMap         *DeviceStateManager
}

func NewStorage(db *gorm.DB, cfg *config.Config, deviceStates *DeviceStateManager) (*Storage, error) {
	s := &Storage{
		db:                db,
		buffer:            []*model.SensorEventModel{},
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
	// Load last recorded date per device from sensor_event
	for _, device := range s.deviceMap.GetAll() {
		lastEvents, err := gorm.G[model.SensorHistoryModel](s.db).
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
		//slog.Debug("stored sensor", "r", r)
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

func (s *Storage) StoreDaily(e *event.SensorEvent, deviceId uint) {
	eventYesterday := time.Time(e.Time).AddDate(0, 0, -1)
	eventYesterdayStr := eventYesterday.Format(time.DateOnly)
	if eventYesterdayStr == s.lastHistory[deviceId] {
		return
	}

	history := model.SensorHistoryModel{}
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

func (s *Storage) Shutdown() {
	close(s.bufferFlushStream)
	s.Flush()
}
