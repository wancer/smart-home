package model

import "time"

const (
	NotifyOffline = "offline" // device went offline (and stayed offline for Duration)
	NotifyOnline  = "online"  // device came back after being offline for at least Duration
	NotifyAbove   = "above"   // Metric > Threshold for Duration
	NotifyBelow   = "below"   // Metric < Threshold for Duration

	MetricPower       = "power"       // W
	MetricCurrent     = "current"     // A
	MetricVoltage     = "voltage"     // V
	MetricCO2         = "co2"         // ppm
	MetricTemperature = "temperature" // °C
	MetricHumidity    = "humidity"    // %
)

// MetricsBySensorType lists the metrics a threshold rule may watch for each device type.
var MetricsBySensorType = map[string][]string{
	SensorTypeEnergy:    {MetricPower, MetricCurrent, MetricVoltage},
	SensorTypeCo2:       {MetricCO2, MetricTemperature, MetricHumidity},
	SensorTypeTempHumid: {MetricTemperature, MetricHumidity},
}

// NotifyRule is a per-device Telegram notification setting.
type NotifyRule struct {
	ID             uint    `gorm:"primaryKey"`
	DeviceId       uint    `gorm:"not null;index"`
	Kind           string  `gorm:"size:16;not null"`
	Metric         string  `gorm:"size:16"` // above/below only
	Threshold      float64 // above/below only
	DurationSec    int     `gorm:"not null;default:0"`
	NotifyRecovery bool    `gorm:"not null;default:false"` // above/below: also notify when the value is back to normal
	Message        string  `gorm:"size:255"`               // optional text replacing the default alert
	Enabled        bool    `gorm:"not null;default:true"`
}

func (NotifyRule) TableName() string { return "notify_rule" }

func (r NotifyRule) Duration() time.Duration { return time.Duration(r.DurationSec) * time.Second }
