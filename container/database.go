package container

import (
	"context"
	"fmt"
	"log/slog"
	"smart-home/config"
	"smart-home/model"

	"gorm.io/gorm"
)

type DatabaseInitializer struct {
	db *gorm.DB
}

func NewDatabaseInitializer(db *gorm.DB) *DatabaseInitializer {
	return &DatabaseInitializer{db: db}
}

func (init *DatabaseInitializer) migrate() error {
	if err := init.db.AutoMigrate(&model.Device{}); err != nil {
		return err
	}
	if err := init.db.AutoMigrate(&model.SensorEvent{}); err != nil {
		return err
	}
	if err := init.db.AutoMigrate(&model.SensorHistory{}); err != nil {
		return err
	}
	return nil
}

func (init *DatabaseInitializer) syncDevices(cfgDevices []config.Device) ([]*model.Device, error) {
	ctx := context.Background()
	dbDevices, err := gorm.G[model.Device](init.db).Find(ctx)
	if err != nil {
		return nil, err
	}

	match := func(cfgDevice config.Device, dbDevices []model.Device) *model.Device {
		for _, dbDevice := range dbDevices {
			if cfgDevice.Topic == dbDevice.Topic {
				return &dbDevice
			}
		}
		return nil
	}

	mappedDevices := []*model.Device{}
	for _, cfgDevice := range cfgDevices {
		dbDevice := match(cfgDevice, dbDevices)
		if dbDevice == nil {
			dbDevice = &model.Device{}
			dbDevice.Topic = cfgDevice.Topic
			dbDevice.Name = cfgDevice.Name
			if err := init.db.Create(&dbDevice).Error; err != nil {
				return nil, err
			}

			slog.Info(fmt.Sprintf("Stored %s device", dbDevice.Topic))
		}

		mappedDevices = append(mappedDevices, dbDevice)
	}

	return mappedDevices, nil
}
