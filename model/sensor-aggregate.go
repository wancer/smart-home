package model

import "time"

// SensorAggregate is one bucket of a device's readings. BucketSeconds is the bucket size
// (0 = a raw reading); a device's rows can have different sizes, see StorageTier.
// Min/max are nil on rows written before they were tracked.
type SensorAggregate struct {
	ID            uint      `gorm:"primaryKey"`
	DeviceId      uint      `gorm:"not null;uniqueIndex:idx_agg_device_time"`
	Device        Device    `gorm:"not null"`
	BucketTime    time.Time `gorm:"not null;uniqueIndex:idx_agg_device_time"`
	BucketSeconds uint      `gorm:"not null;default:300"`
	Count         uint      `gorm:"not null"`

	PowerConsumed *float32 // Wh
	PowerAvg      *float32
	PowerMin      *float32
	PowerMax      *float32
	CurrentAvg    *float32
	CurrentMin    *float32
	CurrentMax    *float32
	VoltageAvg    *float32
	VoltageMin    *float32
	VoltageMax    *float32

	CO2Avg *float32
	CO2Min *float32
	CO2Max *float32

	TemperatureAvg *float32
	TemperatureMin *float32
	TemperatureMax *float32
	HumidityAvg    *float32
	HumidityMin    *float32
	HumidityMax    *float32
	DewPointAvg    *float32
	PressureAvg    *float32 // hPa
	PressureMin    *float32
	PressureMax    *float32

	// DC power monitor (INA219)
	DcVoltageAvg *float32
	DcVoltageMin *float32
	DcVoltageMax *float32
	DcCurrentAvg *float32
	DcCurrentMin *float32
	DcCurrentMax *float32
	DcPowerAvg   *float32
	DcPowerMin   *float32
	DcPowerMax   *float32

	// ESP32 chip temperature, only for devices with StoreChipTemp; pruned after ChipTemperatureRetention.
	ChipTemperatureAvg *float32
}

func (SensorAggregate) TableName() string {
	return "sensor_aggregate"
}
