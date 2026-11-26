package model

import "time"

// Sensor types, detected from the device profile (see internal.DetectSensorType).
const (
	SensorTypeEnergy    = "energy"
	SensorTypeCo2       = "co2"
	SensorTypeTempHumid = "t-h"
	SensorTypeIR        = "ir"      // infra-red remote: no sensors, no power control
	SensorTypeLight     = "light"   // light controller (WS2812 / PWM in the template), no sensors
	SensorTypeSwitch    = "switch"  // relays only (wall switch, relay board): no sensors, no light
	SensorTypeLeak      = "leak"    // water leak sensor (Leak in telemetry), e.g. IKEA KLIPPBOK via a Matter bridge
	SensorTypeContact   = "contact" // door/window contact sensor (Contact in telemetry: 1 = closed)
	SensorTypeOther     = "other"   // nothing recognised (yet)
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
	// PlatformMatter is an end device (sensor) behind a custom Matter bridge (ESP32-S3 + ESP32-C6).
	// Per sensor (Topic): tele/<topic>/SENSOR (flat fields + "Available"), tele/<topic>/INFO,
	// cmnd/<topic>/Identify -> stat/<topic>/RESULT. Shared by every sensor of the bridge
	// (Bridge): tele/<bridge>/LWT and STATE. Only Identify is ever sent to it. Detected from
	// "Available" or set in devices admin.
	PlatformMatter = "matter"
)

// DefaultMatterBridge is the bridge topic of a Matter device that has none set (the bridge's
// LEAK_TOPIC default).
const DefaultMatterBridge = "matter-bridge"

// ValidPlatform reports whether p is a known platform.
func ValidPlatform(p string) bool {
	return p == PlatformTasmota || p == PlatformEsphome || p == PlatformMatter
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
	// Platform is the device firmware (PlatformTasmota, PlatformEsphome, PlatformMatter).
	Platform string `gorm:"size:16;not null;default:tasmota"`
	// Bridge is the MQTT topic of the bridge a Matter device sits behind (its LWT and STATE);
	// "" = DefaultMatterBridge. Unused on other platforms.
	Bridge string `gorm:"size:255;not null;default:''"`
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
	return d.Platform == PlatformTasmota || d.Platform == ""
}

// IsBridged: an end device behind a bridge (Matter): its LWT is the bridge's, its availability
// is the "Available" field of its telemetry.
func (d *Device) IsBridged() bool {
	return d.Platform == PlatformMatter
}

// BridgeTopic is the topic of the bridge of a Matter device (Bridge or the default).
func (d *Device) BridgeTopic() string {
	if d.Bridge == "" {
		return DefaultMatterBridge
	}
	return d.Bridge
}

// AvailabilityFromLwt: devices without sensor data (IR remotes, lights, switches, unrecognised devices)
// come online with LWT "Online"; the others with their first sensor telemetry. Bridged devices
// never: their LWT is the bridge's.
func (d *Device) AvailabilityFromLwt() bool {
	if d.IsBridged() {
		return false
	}
	return d.SensorType == SensorTypeIR || d.SensorType == SensorTypeLight || d.SensorType == SensorTypeSwitch ||
		d.SensorType == SensorTypeOther
}

func (Device) TableName() string {
	return "device"
}
