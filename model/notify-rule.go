package model

import "time"

const (
	NotifyOffline = "offline" // device went offline (and stayed offline for Duration)
	NotifyOnline  = "online"  // device came back after being offline for at least Duration
	NotifyAbove   = "above"   // the series' value > Threshold for Duration
	NotifyBelow   = "below"   // the series' value < Threshold for Duration
)

// NotifyRule is a per-device Telegram notification setting.
type NotifyRule struct {
	ID       uint   `gorm:"primaryKey"`
	DeviceId uint   `gorm:"not null;index"`
	Kind     string `gorm:"size:16;not null"`
	// SeriesId is the watched series (above/below only).
	SeriesId       *uint
	Threshold      float64 // above/below only
	DurationSec    int     `gorm:"not null;default:0"`
	NotifyRecovery bool    `gorm:"not null;default:false"` // above/below: also notify when the value is back to normal; offline: when the device is back online
	Message        string  `gorm:"size:255"`               // optional text replacing the default alert
	Enabled        bool    `gorm:"not null;default:true"`
}

func (NotifyRule) TableName() string { return "notify_rule" }

func (r NotifyRule) Duration() time.Duration { return time.Duration(r.DurationSec) * time.Second }

// UsesThreshold reports whether the rule watches a series value (above/below).
func (r NotifyRule) UsesThreshold() bool {
	return r.Kind == NotifyAbove || r.Kind == NotifyBelow
}
