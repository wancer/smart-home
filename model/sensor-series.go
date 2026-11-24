package model

import "time"

// SensorSeries is one measured value of a device: device × sensor (Source, e.g. BH1750, ENERGY,
// ESP32, MQ, or SourceLegacy for migrated data whose sensor isn't known) × Field (e.g.
// Illuminance). Kind is the physical quantity (see SeriesKind). Rows are created on first sight.
type SensorSeries struct {
	Id        uint      `gorm:"primaryKey"`
	DeviceId  uint      `gorm:"not null;uniqueIndex:idx_series_dsf,priority:1"`
	Source    string    `gorm:"size:64;not null;uniqueIndex:idx_series_dsf,priority:2"`
	Field     string    `gorm:"size:64;not null;uniqueIndex:idx_series_dsf,priority:3"`
	Kind      string    `gorm:"size:32;not null"`
	Label     *string   `gorm:"size:255"` // user-set name, nil = derived from kind/source
	Hidden    bool      `gorm:"not null;default:false"`
	FirstSeen time.Time `gorm:"not null"`
	LastSeen  time.Time `gorm:"not null"`
}

func (SensorSeries) TableName() string {
	return "sensor_series"
}

// LiveOnly reports whether the series keeps only its live value, no history (IsLiveOnly).
func (s SensorSeries) LiveOnly() bool {
	return IsLiveOnly(s.Kind)
}

// SensorValue is one bucket of a series. BucketTime is the bucket start, always written as UTC
// (migrated rows keep their stored text, which may carry another offset: compare with
// datetime() on both sides). BucketSeconds is the bucket size (0 = a raw reading). Avg is
// weighted by Count when buckets merge (internal.MergeValue). Sum is only used by power
// series: the energy (Wh) of the bucket.
type SensorValue struct {
	Id            uint      `gorm:"primaryKey"`
	SeriesId      uint      `gorm:"not null;uniqueIndex:idx_value_series_bucket,priority:1"`
	BucketTime    time.Time `gorm:"not null;uniqueIndex:idx_value_series_bucket,priority:2"`
	BucketSeconds uint      `gorm:"not null"`
	Count         uint      `gorm:"not null"`
	Avg           float64   `gorm:"not null"`
	Min           float64   `gorm:"not null"`
	Max           float64   `gorm:"not null"`
	Sum           *float64
}

func (SensorValue) TableName() string {
	return "sensor_value"
}
