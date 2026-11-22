package internal

import (
	"smart-home/model"
	"time"
)

// Every value in sensor_aggregate merges exactly: sums add, averages are weighted by
// Count, min/max take the smaller/larger one. So a reading, a 5-minute bucket and a
// 1-hour bucket are all the same thing, and any of them can be merged into a coarser
// bucket (flush, rollup, chart steps) without losing anything.

// BucketTime is the start of the step-seconds bucket holding t (step 0 = the reading's
// own second). Buckets line up with UTC hours and days.
func BucketTime(t time.Time, step uint) time.Time {
	if step == 0 {
		return t.UTC().Truncate(time.Second)
	}
	return t.UTC().Truncate(time.Duration(step) * time.Second)
}

// readingAggregate turns one reading into a one-count aggregate.
func readingAggregate(r *model.SensorEvent) *model.SensorAggregate {
	agg := &model.SensorAggregate{
		DeviceId:   r.DeviceId,
		BucketTime: BucketTime(r.RealTime, 0),
		Count:      1,
	}
	if r.Power != nil {
		agg.PowerAvg, agg.PowerMin, agg.PowerMax = triple(float32(*r.Power))
		consumed := float32(0)
		if r.EnergyWh != nil {
			consumed = *r.EnergyWh
		}
		agg.PowerConsumed = &consumed
	}
	if r.Current != nil {
		agg.CurrentAvg, agg.CurrentMin, agg.CurrentMax = triple(*r.Current)
	}
	if r.Voltage != nil {
		agg.VoltageAvg, agg.VoltageMin, agg.VoltageMax = triple(float32(*r.Voltage))
	}
	if r.CO2 != nil {
		agg.CO2Avg, agg.CO2Min, agg.CO2Max = triple(float32(*r.CO2))
	}
	if r.Temperature != nil {
		agg.TemperatureAvg, agg.TemperatureMin, agg.TemperatureMax = triple(*r.Temperature)
	}
	if r.Humidity != nil {
		agg.HumidityAvg, agg.HumidityMin, agg.HumidityMax = triple(*r.Humidity)
	}
	if r.DewPoint != nil {
		agg.DewPointAvg = new(*r.DewPoint)
	}
	if r.Pressure != nil {
		agg.PressureAvg, agg.PressureMin, agg.PressureMax = triple(*r.Pressure)
	}
	if r.Gas != nil {
		agg.GasAvg, agg.GasMin, agg.GasMax = triple(*r.Gas)
	}
	if r.DcVoltage != nil {
		agg.DcVoltageAvg, agg.DcVoltageMin, agg.DcVoltageMax = triple(*r.DcVoltage)
	}
	if r.DcCurrent != nil {
		agg.DcCurrentAvg, agg.DcCurrentMin, agg.DcCurrentMax = triple(*r.DcCurrent)
	}
	if r.DcPower != nil {
		agg.DcPowerAvg, agg.DcPowerMin, agg.DcPowerMax = triple(*r.DcPower)
	}
	if r.ChipTemperature != nil {
		agg.ChipTemperatureAvg = new(*r.ChipTemperature)
	}
	return agg
}

func triple(v float32) (*float32, *float32, *float32) {
	return new(v), new(v), new(v)
}

// groupReadings merges readings into step-second buckets (see BucketTime), keyed by
// bucket start.
func groupReadings(events []*model.SensorEvent, step uint) map[time.Time]*model.SensorAggregate {
	buckets := map[time.Time]*model.SensorAggregate{}
	for _, e := range events {
		agg := readingAggregate(e)
		key := BucketTime(e.RealTime, step)
		if b, ok := buckets[key]; ok {
			MergeInto(b, agg)
			continue
		}
		agg.BucketTime = key
		agg.BucketSeconds = step
		buckets[key] = agg
	}
	return buckets
}

// MergeInto adds src to dst. dst keeps its ID, device and bucket time; its bucket size
// becomes the larger of the two.
func MergeInto(dst, src *model.SensorAggregate) {
	n1 := float32(dst.Count)
	n2 := float32(src.Count)

	dst.BucketSeconds = max(dst.BucketSeconds, src.BucketSeconds)

	dst.PowerConsumed = mergeSum(dst.PowerConsumed, src.PowerConsumed)
	dst.PowerAvg = mergeAvg(dst.PowerAvg, n1, src.PowerAvg, n2)
	dst.PowerMin = mergeMin(dst.PowerMin, src.PowerMin)
	dst.PowerMax = mergeMax(dst.PowerMax, src.PowerMax)
	dst.CurrentAvg = mergeAvg(dst.CurrentAvg, n1, src.CurrentAvg, n2)
	dst.CurrentMin = mergeMin(dst.CurrentMin, src.CurrentMin)
	dst.CurrentMax = mergeMax(dst.CurrentMax, src.CurrentMax)
	dst.VoltageAvg = mergeAvg(dst.VoltageAvg, n1, src.VoltageAvg, n2)
	dst.VoltageMin = mergeMin(dst.VoltageMin, src.VoltageMin)
	dst.VoltageMax = mergeMax(dst.VoltageMax, src.VoltageMax)

	dst.CO2Avg = mergeAvg(dst.CO2Avg, n1, src.CO2Avg, n2)
	dst.CO2Min = mergeMin(dst.CO2Min, src.CO2Min)
	dst.CO2Max = mergeMax(dst.CO2Max, src.CO2Max)

	dst.TemperatureAvg = mergeAvg(dst.TemperatureAvg, n1, src.TemperatureAvg, n2)
	dst.TemperatureMin = mergeMin(dst.TemperatureMin, src.TemperatureMin)
	dst.TemperatureMax = mergeMax(dst.TemperatureMax, src.TemperatureMax)
	dst.HumidityAvg = mergeAvg(dst.HumidityAvg, n1, src.HumidityAvg, n2)
	dst.HumidityMin = mergeMin(dst.HumidityMin, src.HumidityMin)
	dst.HumidityMax = mergeMax(dst.HumidityMax, src.HumidityMax)
	dst.DewPointAvg = mergeAvg(dst.DewPointAvg, n1, src.DewPointAvg, n2)
	dst.PressureAvg = mergeAvg(dst.PressureAvg, n1, src.PressureAvg, n2)
	dst.PressureMin = mergeMin(dst.PressureMin, src.PressureMin)
	dst.PressureMax = mergeMax(dst.PressureMax, src.PressureMax)
	dst.GasAvg = mergeAvg(dst.GasAvg, n1, src.GasAvg, n2)
	dst.GasMin = mergeMin(dst.GasMin, src.GasMin)
	dst.GasMax = mergeMax(dst.GasMax, src.GasMax)

	dst.DcVoltageAvg = mergeAvg(dst.DcVoltageAvg, n1, src.DcVoltageAvg, n2)
	dst.DcVoltageMin = mergeMin(dst.DcVoltageMin, src.DcVoltageMin)
	dst.DcVoltageMax = mergeMax(dst.DcVoltageMax, src.DcVoltageMax)
	dst.DcCurrentAvg = mergeAvg(dst.DcCurrentAvg, n1, src.DcCurrentAvg, n2)
	dst.DcCurrentMin = mergeMin(dst.DcCurrentMin, src.DcCurrentMin)
	dst.DcCurrentMax = mergeMax(dst.DcCurrentMax, src.DcCurrentMax)
	dst.DcPowerAvg = mergeAvg(dst.DcPowerAvg, n1, src.DcPowerAvg, n2)
	dst.DcPowerMin = mergeMin(dst.DcPowerMin, src.DcPowerMin)
	dst.DcPowerMax = mergeMax(dst.DcPowerMax, src.DcPowerMax)

	dst.ChipTemperatureAvg = mergeAvg(dst.ChipTemperatureAvg, n1, src.ChipTemperatureAvg, n2)

	dst.Count += src.Count
}

func mergeSum(a, b *float32) *float32 {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return new(*a + *b)
}

func mergeAvg(a *float32, na float32, b *float32, nb float32) *float32 {
	if a == nil {
		return b
	}
	if b == nil || na+nb == 0 {
		return a
	}
	return new((*a*na + *b*nb) / (na + nb))
}

func mergeMin(a, b *float32) *float32 {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return new(min(*a, *b))
}

func mergeMax(a, b *float32) *float32 {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return new(max(*a, *b))
}
