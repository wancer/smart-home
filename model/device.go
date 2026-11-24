package model

import "time"

// Sensor types, detected from the device profile (see internal.DetectSensorType).
const (
	SensorTypeEnergy    = "energy"
	SensorTypeCo2       = "co2"
	SensorTypeTempHumid = "t-h"
	SensorTypeIR        = "ir"    // infra-red remote: no sensors, no power control
	SensorTypeOther     = "other" // nothing recognised (yet)
	// SensorTypeMulti is any mix of supported sensors (e.g. BME280 + INA219). Every
	// device stores whatever its telemetry reports; the type picks the UI and metrics.
	SensorTypeMulti = "multi"
)

// Platforms: the firmware a device runs, set in devices admin.
const (
	// PlatformTasmota is polled and controlled over cmnd/<topic>/...
	PlatformTasmota = "tasmota"
	// PlatformEsphome is configured in its YAML and only publishes Tasmota-shaped
	// tele/<topic>/SENSOR, STATE and LWT. Nothing is ever sent to it.
	PlatformEsphome = "esphome"
)

// ValidPlatform reports whether p is a known platform.
func ValidPlatform(p string) bool {
	return p == PlatformTasmota || p == PlatformEsphome
}

// DeviceTemplate is the Tasmota GPIO template (reply to the `Template` command).
type DeviceTemplate struct {
	Name string `json:"name"`
	Gpio []int  `json:"gpio"` // Tasmota component codes: function id * 32 + index
	Flag int    `json:"flag"`
	Base int    `json:"base"`
}

type Device struct {
	ID      uint   `gorm:"primaryKey"`
	Name    string `gorm:"size:255;not null"`
	Topic   string `gorm:"uniqueIndex;size:255;not null"`
	Enabled bool   `gorm:"not null;default:true"`
	// Platform is the device firmware (PlatformTasmota, PlatformEsphome).
	Platform string `gorm:"size:16;not null;default:tasmota"`
	// SensorType is detected from the profile; new devices start as SensorTypeOther, set explicitly
	// on create (the column default is a leftover, changing it would make GORM rebuild the table).
	SensorType string `gorm:"default:energy"`
	// OfflineTimeout is how long (seconds) without telemetry marks the device offline;
	// 0 = auto: max(1 min, 2×TelePeriod + 30 s). LWT "Offline" always applies right away.
	OfflineTimeout uint `gorm:"not null;default:0"`
	// StorageTiers is the sensor data retention (see StorageTier); empty = DefaultStorageTiers.
	StorageTiers []StorageTier `gorm:"type:text;serializer:json"`
	// Dashboard is what the dashboard shows for the device; nil = auto.
	Dashboard *DashboardView `gorm:"type:text;serializer:json"`

	// Profile: what the device is, read from it (Template, STATUS2, STATUS10) and kept across
	// restarts. Refreshed whenever the device comes online and on demand.
	Template        *DeviceTemplate `gorm:"type:text;serializer:json"`
	Chip            string          // STATUS2 Hardware, e.g. "ESP8266EX", "ESP32-D0WD-V3"
	FirmwareVersion string          // e.g. "14.3.0(tasmota)"
	FirmwareBuiltAt *time.Time
	FirmwareCore    string   // STATUS2 Core, e.g. "3.3.8" (ESP32 Arduino core)
	FirmwareSdk     string   // STATUS2 SDK, e.g. "5.5.4.260407" (ESP-IDF / NONOS SDK)
	Sensors         []string `gorm:"type:text;serializer:json"` // sensor names in telemetry, e.g. ENERGY, SCD40, ESP32
	ProfiledAt      *time.Time

	// Mq configures an MQ-xxx gas sensor on an analog input; nil = never set up.
	Mq *MqConfig `gorm:"type:text;serializer:json"`
}

// Tiers returns the device's storage tiers, or the default ones.
func (d *Device) Tiers() []StorageTier {
	if len(d.StorageTiers) == 0 {
		return DefaultStorageTiers
	}
	return d.StorageTiers
}

// IsTasmota: the device takes Tasmota commands (polling, control, profile, LED, timers, ...).
func (d *Device) IsTasmota() bool {
	return d.Platform != PlatformEsphome
}

// AvailabilityFromLwt: devices without sensor data (IR remotes, unrecognised devices) come
// online with LWT "Online"; the others with their first sensor telemetry.
func (d *Device) AvailabilityFromLwt() bool {
	return d.SensorType == SensorTypeIR || d.SensorType == SensorTypeOther
}

func (Device) TableName() string {
	return "device"
}
