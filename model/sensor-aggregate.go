package model

import "time"

type SensorAggregate struct {
	ID         uint      `gorm:"primaryKey"`
	DeviceId   uint      `gorm:"not null;uniqueIndex:idx_agg_device_time"`
	Device     Device    `gorm:"not null"`
	BucketTime time.Time `gorm:"not null;uniqueIndex:idx_agg_device_time"`
	Count      uint      `gorm:"not null"`

	PowerConsumed *float32
	PowerAvg      *float32
	CurrentAvg    *float32
	VoltageAvg    *float32

	CO2Avg  *float32
	CO2eAvg *float32

	TemperatureAvg *float32
	HumidityAvg    *float32
	DewPointAvg    *float32
}

func (SensorAggregate) TableName() string {
	return "sensor_aggregate"
}
