package model

import "time"

type SensorEvent struct {
	ID         uint      `gorm:"primaryKey"`
	DeviceId   uint      `gorm:"not null;index:idx_device_time,priority:2"`
	Device     Device    `gorm:"not null"`
	RealTime   time.Time `gorm:"not null;index:idx_device_time,priority:1"`
	DeviceTime time.Time `gorm:"not null"`

	// Energy
	Period        *uint
	Power         *uint
	ApparentPower *uint
	ReactivePower *uint
	Current       *float32
	Voltage       *uint
	// CO2 + TempHumidity
	CO2         *uint
	Temperature *float32
	Humidity    *float32
	DewPoint    *float32

	// ChipTemperature is only buffered for sensor_aggregate; sensor_event is no longer written.
	ChipTemperature *float32 `gorm:"-"`
	// EnergyWh is the energy this power reading stands for (set by Storage.Store).
	EnergyWh *float32 `gorm:"-"`
}

func (SensorEvent) TableName() string {
	return "sensor_event"
}
