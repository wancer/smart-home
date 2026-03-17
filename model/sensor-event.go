package model

import "time"

type SensorEventModel struct {
	ID            uint        `gorm:"primaryKey"`
	DeviceId      uint        `gorm:"not null"`
	Device        DeviceModel `gorm:"not null"`
	RealTime      time.Time   `gorm:"not null"`
	DeviceTime    time.Time   `gorm:"not null"`
	Period        uint        `gorm:"not null"`
	Power         uint        `gorm:"not null"`
	ApparentPower uint        `gorm:"not null"`
	ReactivePower uint        `gorm:"not null"`
	Current       float32     `gorm:"not null"`
}

func (SensorEventModel) TableName() string {
	return "sensor_event"
}
