package internal

import (
	"smart-home/model"
	"testing"
	"time"
)

func nearValue(t *testing.T, name string, got, want float64) {
	t.Helper()
	if got-want > 1e-6 || want-got > 1e-6 {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

// Two readings of several series in one 5-minute bucket: one bucket per series, with
// count-weighted avg, min and max; no sum unless the readings have one.
func TestGroupValuesPerSeries(t *testing.T) {
	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	const pressure, volts, amps, watts = 1, 2, 3, 4
	reading := func(offset time.Duration, series uint, v float64) BufferedValue {
		return BufferedValue{SeriesId: series, DeviceId: 1, At: at.Add(offset), Value: v}
	}
	buckets := groupValues([]BufferedValue{
		reading(0, pressure, 1000), reading(0, volts, 5.0), reading(0, amps, 0.2), reading(0, watts, 1.0),
		reading(time.Minute, pressure, 1010), reading(time.Minute, volts, 5.2), reading(time.Minute, amps, 0.4), reading(time.Minute, watts, 2.0),
	}, 300)

	if len(buckets) != 4 {
		t.Fatalf("expected one bucket per series, got %d", len(buckets))
	}
	bySeries := map[uint]*model.SensorValue{}
	for _, b := range buckets {
		if b.Count != 2 || !b.BucketTime.Equal(at) || b.BucketSeconds != 300 {
			t.Fatalf("expected a 300 s bucket of 2 at %v, got %+v", at, b)
		}
		if b.Sum != nil {
			t.Errorf("series %d: unreported sum must stay nil", b.SeriesId)
		}
		bySeries[b.SeriesId] = b
	}
	nearValue(t, "pressure avg", bySeries[pressure].Avg, 1005)
	nearValue(t, "pressure min", bySeries[pressure].Min, 1000)
	nearValue(t, "pressure max", bySeries[pressure].Max, 1010)
	nearValue(t, "volts avg", bySeries[volts].Avg, 5.1)
	nearValue(t, "amps max", bySeries[amps].Max, 0.4)
	nearValue(t, "watts min", bySeries[watts].Min, 1.0)
}

// Readings in different buckets stay apart; the raw step keeps each second.
func TestGroupValuesBuckets(t *testing.T) {
	at := time.Date(2026, 11, 20, 12, 4, 59, 500_000_000, time.UTC)
	values := []BufferedValue{
		{SeriesId: 1, At: at, Value: 1},
		{SeriesId: 1, At: at.Add(time.Second), Value: 3},
	}
	if got := len(groupValues(values, 300)); got != 2 {
		t.Errorf("5m step: expected 2 buckets across 12:05, got %d", got)
	}
	raw := groupValues(values, 0)
	if len(raw) != 2 || !raw[0].BucketTime.Equal(at.Truncate(time.Second)) || raw[0].BucketSeconds != 0 {
		t.Errorf("raw step: expected one bucket per second, got %+v", raw)
	}
}

// Power energy sums add; avg is weighted by count; the bucket size becomes the larger one.
func TestMergeValue(t *testing.T) {
	dst := &model.SensorValue{Count: 3, Avg: 10, Min: 5, Max: 20, Sum: new(1.5), BucketSeconds: 300}
	src := &model.SensorValue{Count: 1, Avg: 30, Min: 30, Max: 30, Sum: new(0.5), BucketSeconds: 3600}
	MergeValue(dst, src)
	if dst.Count != 4 || dst.BucketSeconds != 3600 {
		t.Fatalf("count/bucket = %d/%d", dst.Count, dst.BucketSeconds)
	}
	nearValue(t, "avg", dst.Avg, 15)
	nearValue(t, "min", dst.Min, 5)
	nearValue(t, "max", dst.Max, 30)
	if dst.Sum == nil {
		t.Fatal("sum lost")
	}
	nearValue(t, "sum", *dst.Sum, 2)

	// a nil sum on either side keeps the other one
	a := &model.SensorValue{Count: 1, Avg: 1, Min: 1, Max: 1}
	MergeValue(a, &model.SensorValue{Count: 1, Avg: 3, Min: 3, Max: 3, Sum: new(2.0)})
	if a.Sum == nil || *a.Sum != 2 {
		t.Errorf("sum = %v, want 2", a.Sum)
	}
	nearValue(t, "avg", a.Avg, 2)
}
