package container

import (
	"context"
	"log/slog"
	"smart-home/model"

	"gorm.io/gorm"
)

type DatabaseInitializer struct {
	db *gorm.DB
}

func NewDatabaseInitializer(db *gorm.DB) *DatabaseInitializer {
	return &DatabaseInitializer{db: db}
}

func (init *DatabaseInitializer) migrate() error {
	if err := init.db.AutoMigrate(&model.Device{}); err != nil {
		return err
	}
	if err := init.db.AutoMigrate(&model.SensorEvent{}); err != nil {
		return err
	}
	if err := init.db.AutoMigrate(&model.SensorHistory{}); err != nil {
		return err
	}
	if err := init.db.AutoMigrate(&model.SensorAggregate{}); err != nil {
		return err
	}
	if err := init.backfillAggregates(); err != nil {
		return err
	}
	return nil
}

// backfillAggregates populates sensor_aggregate from sensor_event on first run.
// It groups raw 15-sec records into 5-minute UTC buckets using SQLite's datetime()
// so that mixed +00:00/+03:00 offsets in historical data are normalised correctly.
func (init *DatabaseInitializer) backfillAggregates() error {
	var aggCount int64
	if err := init.db.Model(&model.SensorAggregate{}).Count(&aggCount).Error; err != nil {
		return err
	}
	if aggCount > 0 {
		return nil // already migrated
	}

	var eventCount int64
	if err := init.db.Model(&model.SensorEvent{}).Count(&eventCount).Error; err != nil {
		return err
	}
	if eventCount == 0 {
		return nil // nothing to migrate
	}

	slog.Info("Backfilling sensor_aggregate from sensor_event", "rows", eventCount)

	sql := `
INSERT INTO sensor_aggregate
    (device_id, bucket_time, count,
     power_consumed, power_avg, current_avg, voltage_avg,
     co2_avg, co2e_avg,
     temperature_avg, humidity_avg, dew_point_avg)
SELECT
    device_id,
    datetime(
        (strftime('%s', datetime(real_time)) / 300) * 300,
        'unixepoch'
    ) AS bucket_time,
    COUNT(*)                                              AS count,
    SUM(CAST(power    AS REAL) * 15.0 / 3600.0)         AS power_consumed,
    AVG(power)                                            AS power_avg,
    AVG(current)                                          AS current_avg,
    AVG(voltage)                                          AS voltage_avg,
    AVG(co2)                                              AS co2_avg,
    AVG(co2e)                                             AS co2e_avg,
    AVG(temperature)                                      AS temperature_avg,
    AVG(humidity)                                         AS humidity_avg,
    AVG(dew_point)                                        AS dew_point_avg
FROM sensor_event
GROUP BY device_id, bucket_time
`
	if err := init.db.Exec(sql).Error; err != nil {
		return err
	}

	var inserted int64
	init.db.Model(&model.SensorAggregate{}).Count(&inserted)
	slog.Info("Backfill complete", "aggregates_written", inserted)
	return nil
}

func (init *DatabaseInitializer) loadDevices() ([]*model.Device, error) {
	ctx := context.Background()
	dbDevices, err := gorm.G[model.Device](init.db).Find(ctx)
	if err != nil {
		return nil, err
	}

	devices := make([]*model.Device, len(dbDevices))
	for i := range dbDevices {
		devices[i] = &dbDevices[i]
	}
	return devices, nil
}
