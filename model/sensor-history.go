package model

import "gorm.io/datatypes"

type SensorHistory struct {
	ID       uint           `gorm:"primaryKey"`
	Device   Device         `gorm:"not null"`
	DeviceId uint           `gorm:"not null"`
	Date     datatypes.Date `gorm:"not null,type:date"`
	Power    float32        `gorm:"not null"`
}

func (SensorHistory) TableName() string {
	return "sensor_history"
}
