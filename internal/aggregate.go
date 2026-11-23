package internal

import (
	"smart-home/model"
	"time"
)

// Every sensor_value row merges exactly: sums add, averages are weighted by Count,
// min/max take the smaller/larger one. So a reading, a 5-minute bucket and a 1-hour
// bucket are all the same thing, and any of them can be merged into a coarser bucket
// (flush, rollup, chart steps) without losing anything. The SQL upsert in upsertValues
// does the same merge in the database.

// BucketTime is the start of the step-seconds bucket holding t (step 0 = the reading's
// own second). Buckets line up with UTC hours and days.
func BucketTime(t time.Time, step uint) time.Time {
	if step == 0 {
		return t.UTC().Truncate(time.Second)
	}
	return t.UTC().Truncate(time.Duration(step) * time.Second)
}

// BufferedValue is one reading of a series, buffered in memory until Storage flushes it
// into sensor_value. Sum is the energy (Wh) a power reading stands for, nil for other kinds.
type BufferedValue struct {
	SeriesId uint
	DeviceId uint
	At       time.Time // when it was received
	Value    float64
	Sum      *float64
}

// readingValue turns one reading into a one-count bucket at its own second.
func readingValue(v *BufferedValue) *model.SensorValue {
	return &model.SensorValue{
		SeriesId:   v.SeriesId,
		BucketTime: BucketTime(v.At, 0),
		Count:      1,
		Avg:        v.Value,
		Min:        v.Value,
		Max:        v.Value,
		Sum:        v.Sum,
	}
}

// groupValues merges readings (of any series) into step-second buckets per series (see
// BucketTime).
func groupValues(values []BufferedValue, step uint) []*model.SensorValue {
	type key struct {
		series uint
		at     time.Time
	}
	buckets := map[key]*model.SensorValue{}
	var result []*model.SensorValue
	for i := range values {
		v := readingValue(&values[i])
		k := key{v.SeriesId, BucketTime(values[i].At, step)}
		if b, ok := buckets[k]; ok {
			MergeValue(b, v)
			continue
		}
		v.BucketTime = k.at
		v.BucketSeconds = step
		buckets[k] = v
		result = append(result, v)
	}
	return result
}

// MergeValue adds src to dst. dst keeps its ID, series and bucket time; its bucket size
// becomes the larger of the two.
func MergeValue(dst, src *model.SensorValue) {
	dst.BucketSeconds = max(dst.BucketSeconds, src.BucketSeconds)
	if n := dst.Count + src.Count; n > 0 {
		dst.Avg = (dst.Avg*float64(dst.Count) + src.Avg*float64(src.Count)) / float64(n)
	}
	dst.Min = min(dst.Min, src.Min)
	dst.Max = max(dst.Max, src.Max)
	dst.Sum = mergeSum(dst.Sum, src.Sum)
	dst.Count += src.Count
}

func mergeSum(a, b *float64) *float64 {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return new(*a + *b)
}
