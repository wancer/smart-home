package internal

import (
	"log/slog"
	"smart-home/model"
	"time"

	"gorm.io/gorm"
)

// rollupBatch is how many rows one rollup transaction reads.
const rollupBatch = 5000

// ApplyStorageTiers merges every device's series into the bucket size of the tier their data
// has aged into, and deletes data older than the last tier (see model.StorageTier). Tiers are
// per device and apply to all its series. Series of devices that no longer exist are left alone.
func (s *Storage) ApplyStorageTiers() {
	now := time.Now().UTC()
	for _, state := range s.deviceMap.GetAll() {
		tiers := state.Device.Tiers()
		for _, seriesId := range s.series.deviceIds(state.Device.ID) {
			if !s.applyTiers(seriesId, tiers, now) {
				break // logged; try again next time
			}
		}
	}
}

// applyTiers works tier by tier: tier i holds data younger than the previous tier's keep
// time, so rows finer than tier i's step that are older than that are merged into tier i's
// buckets. For the first tier that's every finished bucket, which only matters after its
// step was made coarser. Only whole buckets are merged. It reports false on a DB error.
func (s *Storage) applyTiers(seriesId uint, tiers []model.StorageTier, now time.Time) bool {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()
	if !s.series.has(seriesId) {
		return true // deleted meanwhile
	}
	var age time.Duration
	for _, tier := range tiers {
		if tier.Step > 0 {
			cutoff := BucketTime(now.Add(-age), tier.Step)
			if err := s.rollup(seriesId, tier.Step, cutoff); err != nil {
				slog.Error("rollup sensor data", "series_id", seriesId, "step", tier.Step, "err", err)
				return false
			}
		}
		age = tier.KeepDuration()
	}

	if age == 0 {
		return true // the last tier keeps data forever
	}
	cutoff := now.Add(-age).Format(time.DateTime)
	res := s.db.
		Where("series_id = ?", seriesId).
		Where("datetime(bucket_time) < datetime(?)", cutoff).
		Delete(&model.SensorValue{})
	if res.Error != nil {
		slog.Error("delete old sensor data", "series_id", seriesId, "err", res.Error)
		return false
	}
	if res.RowsAffected > 0 {
		slog.Info("Deleted old sensor data", "series_id", seriesId, "rows", res.RowsAffected)
	}
	return true
}

// rollup merges the series' rows finer than step that start before cutoff into step-second
// buckets, in batches.
func (s *Storage) rollup(seriesId uint, step uint, cutoff time.Time) error {
	merged, total := 0, 0
	for {
		var rows []model.SensorValue
		err := s.db.
			Where("series_id = ? AND bucket_seconds < ?", seriesId, step).
			Where("datetime(bucket_time) < datetime(?)", cutoff.Format(time.DateTime)).
			Order("bucket_time ASC").
			Limit(rollupBatch).
			Find(&rows).Error
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}

		buckets := map[time.Time]*model.SensorValue{}
		var ordered []*model.SensorValue
		ids := make([]uint, 0, len(rows))
		for i := range rows {
			row := &rows[i]
			ids = append(ids, row.Id)
			key := BucketTime(row.BucketTime, step)
			if b, ok := buckets[key]; ok {
				MergeValue(b, row)
				continue
			}
			b := *row
			b.Id = 0
			b.BucketTime = key
			b.BucketSeconds = step
			buckets[key] = &b
			ordered = append(ordered, &b)
		}

		// Delete the sources first: one of them may start at the same time as its bucket.
		// A bucket split across batches merges into the row the previous batch wrote.
		err = s.db.Transaction(func(tx *gorm.DB) error {
			for start := 0; start < len(ids); start += 500 {
				if err := tx.Delete(&model.SensorValue{}, ids[start:min(start+500, len(ids))]).Error; err != nil {
					return err
				}
			}
			return upsertValues(tx, ordered)
		})
		if err != nil {
			return err
		}
		total += len(rows)
		merged += len(buckets)
		if len(rows) < rollupBatch {
			break
		}
	}
	if total > 0 {
		slog.Info("Rolled up sensor data", "series_id", seriesId, "step", step, "rows", total, "buckets", merged)
	}
	return nil
}
