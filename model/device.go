package model

const (
	SensorTypeEnergy    = "energy"
	SensorTypeCo2       = "co2"
	SensorTypeTempHumid = "t-h"
)

type Device struct {
	ID             uint   `gorm:"primaryKey"`
	Name           string `gorm:"size:255;not null"`
	Topic          string `gorm:"uniqueIndex;size:255;not null"`
	SensorType     string `gorm:"default:energy"`
	SupportsToggle bool   `gorm:"not null;default:true"`
}

func (Device) TableName() string {
	return "device"
}
