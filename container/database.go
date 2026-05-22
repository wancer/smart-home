package container

import (
	"context"
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

func (init *DatabaseInitializer) loadDevices() ([]*model.Device, error) {
	ctx := context.Background()
	dbDevices, err := gorm.G[model.Device](init.db).Find(ctx)
	if err != nil {
		return nil, err
	}

	devices := make([]*model.Device, len(dbDevices))
	for i := range dbDevices {
		devices[i] = &dbDevices[i]
	}
	return devices, nil
}
