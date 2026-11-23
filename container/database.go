package container

import (
	"context"
	"fmt"
	"log/slog"
	"smart-home/model"

	"gorm.io/gorm"
)

type DatabaseInitializer struct {
	db  *gorm.DB
	dsn string // STORAGE_DSN, for the backup before the series migration
}

func NewDatabaseInitializer(db *gorm.DB, dsn string) *DatabaseInitializer {
	return &DatabaseInitializer{db: db, dsn: dsn}
}

func (init *DatabaseInitializer) migrate() error {
	if err := init.db.AutoMigrate(&model.Device{}); err != nil {
		return err
	}
	if err := init.db.AutoMigrate(&model.SensorHistory{}); err != nil {
		return err
	}
	// sensor_aggregate is no longer auto-migrated: it is only read by migrateSeries, then dropped.
	if err := init.db.AutoMigrate(&model.SensorSeries{}, &model.SensorValue{}); err != nil {
		return err
	}
	if err := init.db.AutoMigrate(&model.IrSignal{}); err != nil {
		return err
	}
	if err := init.db.AutoMigrate(&model.IrRule{}, &model.IrRuleAction{}); err != nil {
		return err
	}
	if err := init.db.AutoMigrate(&model.NotifyRule{}); err != nil {
		return err
	}
	// before dropColumns, so the backup still has every legacy column
	if err := init.backupForSeries(); err != nil {
		return err
	}
	if err := init.addMissingLegacyColumns(); err != nil {
		return err
	}
	if err := init.dropColumns(); err != nil {
		return err
	}
	if err := init.dropTables(); err != nil {
		return err
	}
	return init.migrateSeries()
}

// dropTables removes tables that are no longer used. It is a no-op once they are gone.
// Run VACUUM to shrink the file.
//   - sensor_event: legacy raw readings, copied into sensor_aggregate on the first boot after
//     the aggregates were introduced and never read since
//
// sensor_aggregate is not listed here: migrateSeries (series-migration.go) drops it only after
// its data was copied into sensor_value, verified and backed up.
func (init *DatabaseInitializer) dropTables() error {
	for _, table := range []string{"sensor_event"} {
		if !init.db.Migrator().HasTable(table) {
			continue
		}
		slog.Info("Dropping table", "table", table)
		if err := init.db.Migrator().DropTable(table); err != nil {
			return err
		}
	}
	return nil
}

// dropColumns removes columns that are no longer used. AutoMigrate never drops columns, so
// this is done explicitly. It is a no-op once the columns are gone, and on fresh databases.
// Run VACUUM to shrink the file.
//   - eCO2: Tasmota stopped reporting it in v15.5.0
//   - device.supports_toggle: derived from the relays in the stored template now
//   - device.store_chip_temp: the chip temperature is stored for every device reporting it
//   - gas_*: an MQ ppm value from Tasmota's ANALOG MQ*_1, replaced by gas_ratio_* / gas_rs_*
//   - device.mq_calibration: R0 of the MQ Berry script, replaced by device.mq (computed in the backend)
func (init *DatabaseInitializer) dropColumns() error {
	columns := []struct {
		model  any
		table  string
		column string
	}{
		{&model.SensorAggregate{}, "sensor_aggregate", "co2e_avg"},
		{&model.Device{}, "device", "supports_toggle"},
		{&model.Device{}, "device", "store_chip_temp"},
		{&model.SensorAggregate{}, "sensor_aggregate", "gas_avg"},
		{&model.SensorAggregate{}, "sensor_aggregate", "gas_min"},
		{&model.SensorAggregate{}, "sensor_aggregate", "gas_max"},
		{&model.Device{}, "device", "mq_calibration"},
	}
	for _, c := range columns {
		if !init.db.Migrator().HasColumn(c.model, c.column) {
			continue
		}
		slog.Info("Dropping column", "table", c.table, "column", c.column)
		if err := init.db.Exec("ALTER TABLE " + c.table + " DROP COLUMN " + c.column).Error; err != nil {
			return err
		}
	}
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

// addMissingLegacyColumns adds the model.SensorAggregate columns a very old sensor_aggregate
// lacks (e.g. bucket_seconds, default 300), so the series migration can read them. Only
// ALTER TABLE ADD COLUMN: unlike AutoMigrate, it never rebuilds the table or changes a column.
func (init *DatabaseInitializer) addMissingLegacyColumns() error {
	if !init.db.Migrator().HasTable(legacyTable) {
		return nil
	}
	stmt := &gorm.Statement{DB: init.db}
	if err := stmt.Parse(&model.SensorAggregate{}); err != nil {
		return err
	}
	for _, field := range stmt.Schema.Fields {
		if field.DBName == "" || init.db.Migrator().HasColumn(&model.SensorAggregate{}, field.DBName) {
			continue
		}
		slog.Info("Adding missing legacy column for the series migration", "table", legacyTable, "column", field.DBName)
		if err := init.db.Migrator().AddColumn(&model.SensorAggregate{}, field.Name); err != nil {
			return fmt.Errorf("add column %s.%s: %w", legacyTable, field.DBName, err)
		}
	}
	return nil
}
