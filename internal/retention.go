package internal

import (
	"log/slog"
	"smart-home/model"
	"time"

	"gorm.io/gorm"
)

// rollupBatch is how many rows one rollup transaction reads.
const rollupBatch = 5000

// ApplyStorageTiers merges every device's sensor data into the bucket size of the tier
// it has aged into, and deletes data older than the last tier (see model.StorageTier).
func (s *Storage) ApplyStorageTiers() {
	now := time.Now().UTC()
	for _, state := range s.deviceMap.GetAll() {
		s.applyTiers(state.Device.ID, state.Device.Tiers(), now)
	}
}

// applyTiers works tier by tier: tier i holds data younger than the previous tier's keep
// time, so rows finer than tier i's step that are older than that are merged into tier i's
// buckets. For the first tier that's every finished bucket, which only matters after its
// step was made coarser. Only whole buckets are merged.
func (s *Storage) applyTiers(deviceId uint, tiers []model.StorageTier, now time.Time) {
	var age time.Duration
	for _, tier := range tiers {
		if tier.Step > 0 {
			cutoff := BucketTime(now.Add(-age), tier.Step)
			if err := s.rollup(deviceId, tier.Step, cutoff); err != nil {
				slog.Error("rollup sensor data", "device_id", deviceId, "step", tier.Step, "err", err)
				return
			}
		}
		age = tier.KeepDuration()
	}

	if age == 0 {
		return // the last tier keeps data forever
	}
	cutoff := now.Add(-age).Format(time.DateTime)
	res := s.db.
		Where("device_id = ?", deviceId).
		Where("datetime(bucket_time) < datetime(?)", cutoff).
		Delete(&model.SensorAggregate{})
	if res.Error != nil {
		slog.Error("delete old sensor data", "device_id", deviceId, "err", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		slog.Info("Deleted old sensor data", "device_id", deviceId, "rows", res.RowsAffected)
	}
}

// rollup merges the device's rows finer than step that start before cutoff into
// step-second buckets, in batches.
func (s *Storage) rollup(deviceId uint, step uint, cutoff time.Time) error {
	merged, total := 0, 0
	for {
		var rows []model.SensorAggregate
		err := s.db.
			Where("device_id = ? AND bucket_seconds < ?", deviceId, step).
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

		buckets := map[time.Time]*model.SensorAggregate{}
		ids := make([]uint, 0, len(rows))
		for i := range rows {
			row := &rows[i]
			ids = append(ids, row.ID)
			key := BucketTime(row.BucketTime, step)
			if b, ok := buckets[key]; ok {
				MergeInto(b, row)
				continue
			}
			b := *row
			b.ID = 0
			b.BucketTime = key
			b.BucketSeconds = step
			buckets[key] = &b
		}

		// Delete the sources first: one of them may start at the same time as its bucket.
		// A bucket split across batches merges into the row the previous batch wrote.
		err = s.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Delete(&model.SensorAggregate{}, ids).Error; err != nil {
				return err
			}
			for _, b := range buckets {
				if err := upsertAggregate(tx, b); err != nil {
					return err
				}
			}
			return nil
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
		slog.Info("Rolled up sensor data", "device_id", deviceId, "step", step, "rows", total, "buckets", merged)
	}
	return nil
}
