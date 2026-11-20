package model

const (
	SensorTypeEnergy    = "energy"
	SensorTypeCo2       = "co2"
	SensorTypeTempHumid = "t-h"
	SensorTypeIR        = "ir" // infra-red remote: no sensors, no power control
	// SensorTypeMulti is any mix of supported sensors (e.g. BME280 + INA219). Every
	// device stores whatever its telemetry reports; the type picks the UI and metrics.
	SensorTypeMulti = "multi"
)

type Device struct {
	ID             uint   `gorm:"primaryKey"`
	Name           string `gorm:"size:255;not null"`
	Topic          string `gorm:"uniqueIndex;size:255;not null"`
	Enabled        bool   `gorm:"not null;default:true"`
	SensorType     string `gorm:"default:energy"`
	SupportsToggle bool   `gorm:"not null;default:true"`
	StoreChipTemp  bool   `gorm:"not null;default:false"` // keep the ESP32 chip temperature for 24h
	// OfflineTimeout is how long (seconds) without telemetry marks the device offline;
	// 0 = auto: max(1 min, 2×TelePeriod + 30 s). LWT "Offline" always applies right away.
	OfflineTimeout uint `gorm:"not null;default:0"`
	// StorageTiers is the sensor data retention (see StorageTier); empty = DefaultStorageTiers.
	StorageTiers []StorageTier `gorm:"type:text;serializer:json"`
}

// Tiers returns the device's storage tiers, or the default ones.
func (d *Device) Tiers() []StorageTier {
	if len(d.StorageTiers) == 0 {
		return DefaultStorageTiers
	}
	return d.StorageTiers
}

func (Device) TableName() string {
	return "device"
}
