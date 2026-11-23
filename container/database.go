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
	if err := init.db.AutoMigrate(&model.SensorHistory{}); err != nil {
		return err
	}
	if err := init.db.AutoMigrate(&model.SensorAggregate{}); err != nil {
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
	if err := init.dropColumns(); err != nil {
		return err
	}
	return init.dropTables()
}

// dropTables removes tables that are no longer used. It is a no-op once they are gone.
// Run VACUUM to shrink the file.
//   - sensor_event: legacy raw readings, copied into sensor_aggregate on the first boot after
//     the aggregates were introduced and never read since
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
