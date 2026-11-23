package model

import "time"

// SensorEvent is one reading, buffered in memory until Storage flushes it into
// sensor_aggregate. It is not stored as is (the legacy sensor_event table was dropped).
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

	Pressure  *float32 `gorm:"-"` // hPa
	DcVoltage *float32 `gorm:"-"`
	DcCurrent *float32 `gorm:"-"`
	DcPower   *float32 `gorm:"-"`
	GasRatio  *float32 `gorm:"-"` // MQ-xxx, 1.0 = calibration air
	GasRs     *float32 `gorm:"-"` // MQ-xxx sensor resistance, Ω, uncorrected

	Illuminance *float32 `gorm:"-"` // lx
	ColorTemp   *float32 `gorm:"-"` // K

	ChipTemperature *float32 `gorm:"-"`
	// EnergyWh is the energy this power reading stands for (set by Storage.Store).
	EnergyWh *float32 `gorm:"-"`
}
