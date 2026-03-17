package model

import "gorm.io/datatypes"

type SensorHistoryModel struct {
	ID       uint           `gorm:"primaryKey"`
	Device   DeviceModel    `gorm:"not null"`
	DeviceId uint           `gorm:"not null"`
	Date     datatypes.Date `gorm:"not null,type:date"`
	Power    float32        `gorm:"not null"`
}

func (SensorHistoryModel) TableName() string {
	return "sensor_history"
}
