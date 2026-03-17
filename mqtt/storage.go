package mqtt

import (
	"context"
	"fmt"
	"log/slog"
	"smart-home/config"
	"smart-home/model"
	"sync"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Storage struct {
	db             *gorm.DB
	buffer         []model.SensorEventModel
	devicesByTopic map[string]model.DeviceModel
	lock           sync.Mutex
	lastHistory    map[uint]string
	//	historyToUpdate   map[string]DeviceModel
	bufferFlushStream chan struct{}
}

func NewStorage(db *gorm.DB, cfg *config.Config) (*Storage, error) {
	s := &Storage{
		db:                db,
		buffer:            []model.SensorEventModel{},
		devicesByTopic:    map[string]model.DeviceModel{},
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
	if err := s.db.AutoMigrate(&model.DeviceModel{}); err != nil {
		return err
	}
	if err := s.db.AutoMigrate(&model.SensorEventModel{}); err != nil {
		return err
	}
	if err := s.db.AutoMigrate(&model.SensorHistoryModel{}); err != nil {
		return err
	}

	devices, err := gorm.G[model.DeviceModel](s.db).Find(ctx)
	if err != nil {
		return err
	}

	for _, cfgDevice := range c.Devices {
		found := false
		for _, dbDevice := range devices {
			if cfgDevice.Topic == dbDevice.Topic {
				s.devicesByTopic[dbDevice.Topic] = dbDevice
				found = true
				break
			}
		}

		if !found {
			r := model.DeviceModel{}
			r.Topic = cfgDevice.Topic
			r.Name = cfgDevice.Name
			if err := s.db.Save(&r).Error; err != nil {
				return err
			}

			s.devicesByTopic[r.Topic] = r
			slog.Info(fmt.Sprintf("Stored %s device", r.Topic))
		}
	}

	// Load last recorded date per device from sensor_event
	for deviceTopic, device := range s.devicesByTopic {
		lastEvents, err := gorm.G[model.SensorHistoryModel](s.db).
			Where("device_id = ?", device.ID).
			Order("date DESC").
			Limit(1).
			Find(ctx)
		if err != nil {
			return err
		}

		if len(lastEvents) == 0 {
			slog.Warn("No last record for", "topic", deviceTopic)
			s.lastHistory[device.ID] = ""
		} else {
			lastEvent := lastEvents[0]
			s.lastHistory[device.ID] = time.Time(lastEvent.Date).Format(time.DateOnly)
			slog.Debug("Loaded last history", "topic", deviceTopic, "device_id", device.ID, "last_date", s.lastHistory[device.ID])
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
	s.buffer = []model.SensorEventModel{}
	s.lock.Unlock()

	for _, r := range buffer {
		if err := s.db.Save(&r).Error; err != nil {
			slog.Error(err.Error())
		}
		slog.Debug("stored sensor", "r", r)
	}

	slog.Info(fmt.Sprintf("Stored %d events", len(buffer)))
}

func (s *Storage) GetBuffer() []model.SensorEventModel {
	s.lock.Lock()
	defer s.lock.Unlock()
	buffer := s.buffer

	return buffer
}

func (s *Storage) Store(topic string, e *SensorEvent, now *time.Time) {
	s.lock.Lock()
	defer s.lock.Unlock()

	deviceId := s.devicesByTopic[topic].ID

	r := model.SensorEventModel{}
	r.DeviceId = deviceId
	r.RealTime = *now
	r.DeviceTime = time.Time(e.Time)
	r.Period = uint(e.Energy.Period)
	r.Power = uint(e.Energy.Power)
	r.ApparentPower = uint(e.Energy.ApparentPower)
	r.ReactivePower = uint(e.Energy.ReactivePower)
	r.Current = e.Energy.Current

	s.buffer = append(s.buffer, r)
	slog.Debug("added to buffer", "event", e)

	eventYesterday := time.Time(e.Time).AddDate(0, 0, -1)
	eventYesterdayStr := eventYesterday.Format(time.DateOnly)
	if eventYesterdayStr != s.lastHistory[deviceId] {
		history := model.SensorHistoryModel{}
		history.Date = datatypes.Date(eventYesterday)
		history.DeviceId = deviceId
		history.Power = e.Energy.Yesterday
		if err := s.db.Save(&history).Error; err != nil {
			slog.Error("CANT_SAVE_HISTORY", "err", err)
		} else {
			slog.Info("Save history", "date", eventYesterdayStr, "topic", topic)
			s.lastHistory[deviceId] = eventYesterdayStr
		}
	}
}

func (s *Storage) Shutdown() {
	close(s.bufferFlushStream)
	s.Flush()
}
