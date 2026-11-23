package container

import (
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"smart-home/model"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// One-off migration of the legacy sensor_aggregate table (one row per device and bucket, a
// column per metric) into sensor_series / sensor_value (one row per series and bucket). It runs
// entirely within one start, before Storage exists, so nothing else writes sensor_value meanwhile:
//
//   - Backup (backupForSeries, before dropColumns): VACUUM INTO "<db>.pre-series-<time>.bak"
//     (written as .tmp, then renamed), after checking free space ≥ 5× the DB (+WAL) size. The
//     path is kept in migration_state (series.backup) and reused when a start resumes.
//   - Copy (migrateSeries): rows in id order, migrateBatch rows per transaction; each copies
//     every metric column with INSERT ... SELECT (bucket_time text, bucket_seconds and count as
//     stored) and saves the last copied id (series.last_aggregate_id), so a crash resumes there.
//     A conflicting sensor_value row aborts the batch: nothing should be there.
//   - Verify, right before the drop in the same start: per device and column every non-NULL
//     value has its sensor_value row (same series and bucket_time text), with the same sums of
//     avg/min/max/count/bucket_seconds; power_consumed against the power series' sum; no
//     unmapped column (or min/max without avg) holding values.
//   - Drop: sensor_aggregate and the series.* migration_state rows in one transaction.
//
// No backup (DSN not a file, too little space, VACUUM error) or a copy error: Build fails and the
// service refuses to start, with a log saying what to do. A failed verification keeps the table
// (all data is in both places), persists series.verified = -1 and the service starts; later
// starts only log that, until that row is deleted.
//
// Old columns map to series by source: ENERGY for power/current/voltage, ESP32 for the chip
// temperature, MQ for gas ratio / Rs, else the one sensor in device.sensors known to report
// the quantity (with the old precedence: a T-H sensor before a CO₂ sensor for climate, a
// light meter before a colour sensor for illuminance), or model.SourceLegacy when none or
// several match.

const (
	legacyTable       = "sensor_aggregate"
	stateKeyPrefix    = "series."
	stateKeyProgress  = "series.last_aggregate_id"
	stateKeyVerified  = "series.verified" // only "-1" (failed) is stored
	stateKeyBackup    = "series.backup"   // path of the backup file
	migrateLogBatches = 50
	backupSpaceFactor = 5
)

// migrateBatch is how many sensor_aggregate rows one copy transaction reads (a var for tests).
var migrateBatch = 5000

// legacyColumn is one metric of sensor_aggregate (Go field names of model.SensorAggregate).
type legacyColumn struct {
	name          string // for logs
	avg, min, max string // min/max "" = only the average was stored (min = max = avg)
	sum           string // power_consumed for power
	field         string // series field
	source        func(sensors []string) string
}

func fixed(source string) func([]string) string {
	return func([]string) string { return source }
}

var legacyColumns = []legacyColumn{
	{name: "power", avg: "PowerAvg", min: "PowerMin", max: "PowerMax", sum: "PowerConsumed", field: "Power", source: fixed(model.SourceEnergy)},
	{name: "current", avg: "CurrentAvg", min: "CurrentMin", max: "CurrentMax", field: "Current", source: fixed(model.SourceEnergy)},
	{name: "voltage", avg: "VoltageAvg", min: "VoltageMin", max: "VoltageMax", field: "Voltage", source: fixed(model.SourceEnergy)},
	{name: "co2", avg: "CO2Avg", min: "CO2Min", max: "CO2Max", field: "CarbonDioxide", source: co2Source},
	{name: "temperature", avg: "TemperatureAvg", min: "TemperatureMin", max: "TemperatureMax", field: "Temperature", source: climateSource},
	{name: "humidity", avg: "HumidityAvg", min: "HumidityMin", max: "HumidityMax", field: "Humidity", source: climateSource},
	{name: "dew_point", avg: "DewPointAvg", field: "DewPoint", source: climateSource},
	{name: "pressure", avg: "PressureAvg", min: "PressureMin", max: "PressureMax", field: "Pressure", source: pressureSource},
	{name: "gas_ratio", avg: "GasRatioAvg", min: "GasRatioMin", max: "GasRatioMax", field: "Ratio", source: fixed(model.SourceMq)},
	{name: "gas_rs", avg: "GasRsAvg", min: "GasRsMin", max: "GasRsMax", field: "Rs", source: fixed(model.SourceMq)},
	{name: "dc_voltage", avg: "DcVoltageAvg", min: "DcVoltageMin", max: "DcVoltageMax", field: "Voltage", source: dcSource},
	{name: "dc_current", avg: "DcCurrentAvg", min: "DcCurrentMin", max: "DcCurrentMax", field: "Current", source: dcSource},
	{name: "dc_power", avg: "DcPowerAvg", min: "DcPowerMin", max: "DcPowerMax", field: "Power", source: dcSource},
	{name: "illuminance", avg: "IlluminanceAvg", min: "IlluminanceMin", max: "IlluminanceMax", field: "Illuminance", source: illuminanceSource},
	{name: "color_temp", avg: "ColorTempAvg", min: "ColorTempMin", max: "ColorTempMax", field: "ColorTemperature", source: colorSource},
	{name: "chip_temperature", avg: "ChipTemperatureAvg", field: "Temperature", source: fixed(model.SourceChip)},
}

// Known Tasmota sensor names (prefixes, upper case; instances get suffixes like "-0x44").
var (
	thSensors       = []string{"SHT", "AM23", "DHT", "SI70", "HTU", "BME280", "BME680", "BME688", "AHT", "HDC", "HIH"}
	co2Sensors      = []string{"SCD", "MHZ", "S8", "SENSEAIR"}
	pressureSensors = []string{"BME280", "BME680", "BME688", "BMP", "LPS", "DPS310", "HP303", "SPL06", "MS5837"}
	dcSensors       = []string{"INA2", "INA3"}
	lightMeters     = []string{"BH1750", "TSL2", "VEML7700", "VEML6030", "MAX44009", "LTR"}
	colorSensors    = []string{"TCS34725"}
)

func matching(sensors []string, prefixes []string) []string {
	var found []string
	for _, s := range sensors {
		upper := strings.ToUpper(s)
		for _, p := range prefixes {
			if strings.HasPrefix(upper, p) {
				found = append(found, s)
				break
			}
		}
	}
	return found
}

// single returns the only match, else SourceLegacy.
func single(found []string) string {
	if len(found) == 1 {
		return found[0]
	}
	return model.SourceLegacy
}

func co2Source(sensors []string) string      { return single(matching(sensors, co2Sensors)) }
func pressureSource(sensors []string) string { return single(matching(sensors, pressureSensors)) }
func dcSource(sensors []string) string       { return single(matching(sensors, dcSensors)) }
func colorSource(sensors []string) string    { return single(matching(sensors, colorSensors)) }

// climateSource: temperature/humidity came from a T-H sensor, else from the CO₂ sensor.
func climateSource(sensors []string) string {
	if th := matching(sensors, thSensors); len(th) > 0 {
		return single(th)
	}
	return co2Source(sensors)
}

// illuminanceSource: illuminance came from a light meter, else from the colour sensor.
func illuminanceSource(sensors []string) string {
	if light := matching(sensors, lightMeters); len(light) > 0 {
		return single(light)
	}
	return colorSource(sensors)
}

// presentColumn is a legacyColumn with its DB column names (all present in the table).
type presentColumn struct {
	legacyColumn
	avgCol, minCol, maxCol, sumCol string // minCol/maxCol = avgCol when not stored; sumCol "" = none
}

// backupForSeries writes the backup before anything changes the legacy table (dropColumns,
// the copy). Needed whenever the table is there and not stuck on a failed verification;
// a backup written by an earlier (interrupted) start is reused while its file exists.
func (init *DatabaseInitializer) backupForSeries() error {
	db := init.db
	if !db.Migrator().HasTable(legacyTable) {
		return nil
	}
	if err := ensureStateTable(db); err != nil {
		return err
	}
	if verified, _, err := getState(db, stateKeyVerified); err != nil || verified == "-1" {
		return err // stuck: the table stays, nothing to do
	}
	if path, found, err := getState(db, stateKeyBackup); err != nil {
		return err
	} else if found {
		if _, err := os.Stat(path); err == nil {
			slog.Info("Series migration: using the backup written earlier", "path", path)
			return nil
		}
		slog.Warn("Series migration: the earlier backup is gone, writing a new one", "path", path)
	}

	path, err := init.backup()
	if err != nil {
		slog.Error("SERIES MIGRATION: NO BACKUP, REFUSING TO START. Nothing was changed. "+
			"Free disk space next to the database, or set STORAGE_DSN to the plain database file path, then start again.",
			"err", err)
		return fmt.Errorf("series migration backup: %w", err)
	}
	return setState(db, stateKeyBackup, path)
}

// migrateSeries copies, verifies and drops (see the top of this file). Runs after backupForSeries.
func (init *DatabaseInitializer) migrateSeries() error {
	db := init.db
	if !db.Migrator().HasTable(legacyTable) {
		if db.Migrator().HasTable("migration_state") {
			// leftovers of a migration finished elsewhere (e.g. state restored without the table)
			return db.Exec(`DELETE FROM migration_state WHERE name LIKE ?`, stateKeyPrefix+"%").Error
		}
		return nil
	}
	if err := ensureStateTable(db); err != nil {
		return err
	}
	if verified, _, err := getState(db, stateKeyVerified); err != nil {
		return err
	} else if verified == "-1" {
		slog.Error("SERIES MIGRATION VERIFICATION FAILED on an earlier start (see that log): sensor_aggregate is kept, " +
			"all its data is also in sensor_value. Check the data, then delete the 'series.verified' row of migration_state to migrate/verify again.")
		return nil
	}

	backupPath, found, err := getState(db, stateKeyBackup)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("series migration: no backup recorded (backupForSeries must run first)")
	}

	var maxId uint64
	if err := db.Raw(`SELECT COALESCE(MAX(id), 0) FROM ` + legacyTable).Scan(&maxId).Error; err != nil {
		return err
	}
	progressText, _, err := getState(db, stateKeyProgress)
	if err != nil {
		return err
	}
	var progress uint64
	if progressText != "" {
		if progress, err = strconv.ParseUint(progressText, 10, 64); err != nil {
			return fmt.Errorf("series migration: bad progress %q: %w", progressText, err)
		}
	}

	columns, err := init.presentColumns()
	if err != nil {
		return err
	}
	devices, err := init.deviceSensors()
	if err != nil {
		return err
	}

	if maxId > progress {
		slog.Info("Series migration: copying sensor_aggregate", "from_id", progress, "max_id", maxId, "backup", backupPath)
		if err := init.copyAggregates(columns, devices, progress, maxId); err != nil {
			slog.Error("SERIES MIGRATION COPY FAILED, REFUSING TO START. Every finished batch is saved and the next start resumes. "+
				"If it keeps failing, restore the backup and report the error.", "err", err, "backup", backupPath)
			return fmt.Errorf("series migration copy: %w", err)
		}
	}

	ok, err := init.verifyMigration(columns, devices)
	if err != nil {
		slog.Error("SERIES MIGRATION VERIFICATION COULD NOT RUN, REFUSING TO START (the next start verifies again)", "err", err)
		return fmt.Errorf("series migration verification: %w", err)
	}
	if !ok {
		slog.Error("SERIES MIGRATION VERIFICATION FAILED: sensor_aggregate is kept, nothing was dropped. " +
			"All its data is also in sensor_value; the service starts normally.")
		return setState(db, stateKeyVerified, "-1")
	}

	if _, err := os.Stat(backupPath); err != nil {
		slog.Error("Series migration verified, but the backup file is gone: sensor_aggregate is kept (a new backup is written on the next start)",
			"path", backupPath, "err", err)
		return deleteState(db, stateKeyBackup)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Migrator().DropTable(legacyTable); err != nil {
			return err
		}
		return tx.Exec(`DELETE FROM migration_state WHERE name LIKE ?`, stateKeyPrefix+"%").Error
	})
	if err != nil {
		slog.Error("Series migration: dropping sensor_aggregate failed, it is kept (retried on the next start)", "err", err)
		return nil
	}
	slog.Info("Series migration done, sensor_aggregate dropped. Run VACUUM to shrink the file.", "backup", backupPath)
	return nil
}

// presentColumns returns the legacy columns that exist in the table (an old schema may lack some).
func (init *DatabaseInitializer) presentColumns() ([]presentColumn, error) {
	types, err := init.db.Migrator().ColumnTypes(legacyTable)
	if err != nil {
		return nil, err
	}
	exists := map[string]bool{}
	for _, t := range types {
		exists[t.Name()] = true
	}
	name := func(field string) string {
		if field == "" {
			return ""
		}
		return init.db.NamingStrategy.ColumnName(legacyTable, field)
	}

	var result []presentColumn
	for _, c := range legacyColumns {
		p := presentColumn{legacyColumn: c, avgCol: name(c.avg), minCol: name(c.min), maxCol: name(c.max), sumCol: name(c.sum)}
		if !exists[p.avgCol] {
			slog.Info("Series migration: column not in sensor_aggregate, skipped", "column", p.avgCol)
			continue
		}
		if p.minCol == "" || !exists[p.minCol] {
			p.minCol = p.avgCol
		}
		if p.maxCol == "" || !exists[p.maxCol] {
			p.maxCol = p.avgCol
		}
		if p.sumCol != "" && !exists[p.sumCol] {
			p.sumCol = ""
		}
		result = append(result, p)
	}
	return result, nil
}

// deviceSensors returns the seen sensor names of every device (device.sensors).
func (init *DatabaseInitializer) deviceSensors() (map[uint][]string, error) {
	var devices []model.Device
	if err := init.db.Find(&devices).Error; err != nil {
		return nil, err
	}
	result := map[uint][]string{}
	for _, d := range devices {
		result[d.ID] = d.Sensors
	}
	return result, nil
}

// ensureSeries returns the ID of the device's series, creating it if needed.
func ensureSeries(db *gorm.DB, deviceId uint, source, field string) (uint, error) {
	now := time.Now().UTC()
	row := model.SensorSeries{
		DeviceId: deviceId, Source: source, Field: field, Kind: model.SeriesKind(source, field),
		FirstSeen: now, LastSeen: now,
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return 0, err
	}
	return findSeries(db, deviceId, source, field)
}

// findSeries returns the ID of the device's series, 0 if there is none.
func findSeries(db *gorm.DB, deviceId uint, source, field string) (uint, error) {
	var rows []model.SensorSeries
	err := db.Where("device_id = ? AND source = ? AND field = ?", deviceId, source, field).Limit(1).Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return rows[0].Id, nil
}

// columnCounts returns, per device, the number of rows (key "") and of non-NULL values
// per column (key: column name) among the rows with id > fromId.
func (init *DatabaseInitializer) columnCounts(columns []presentColumn, fromId uint64) (map[uint]map[string]int64, error) {
	sel := []string{"device_id", "COUNT(*)"}
	for _, c := range columns {
		sel = append(sel, "COUNT("+c.avgCol+")")
	}
	rows, err := init.db.Raw(`SELECT `+strings.Join(sel, ", ")+` FROM `+legacyTable+` WHERE id > ? GROUP BY device_id`, fromId).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[uint]map[string]int64{}
	for rows.Next() {
		var deviceId uint
		counts := make([]int64, len(columns)+1)
		dest := []any{&deviceId}
		for i := range counts {
			dest = append(dest, &counts[i])
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		byColumn := map[string]int64{"": counts[0]}
		for i, c := range columns {
			byColumn[c.name] = counts[i+1]
		}
		result[deviceId] = byColumn
	}
	return result, rows.Err()
}

// copyAggregates copies the rows with fromId < id <= toId.
func (init *DatabaseInitializer) copyAggregates(columns []presentColumn, devices map[uint][]string, fromId, toId uint64) error {
	db := init.db
	counts, err := init.columnCounts(columns, fromId)
	if err != nil {
		return err
	}

	// series per column and device: a CASE on device_id per column
	type target struct {
		caseExpr  string
		deviceIds []string
	}
	targets := map[string]*target{}
	for deviceId, byColumn := range counts {
		sensors, known := devices[deviceId]
		if !known {
			slog.Warn("Series migration: sensor_aggregate rows of a deleted device are migrated with legacy sources", "device_id", deviceId)
		}
		var logged []any
		for _, c := range columns {
			if byColumn[c.name] == 0 {
				continue
			}
			source := c.source(sensors)
			seriesId, err := ensureSeries(db, deviceId, source, c.field)
			if err != nil {
				return err
			}
			t := targets[c.name]
			if t == nil {
				t = &target{}
				targets[c.name] = t
			}
			id := strconv.FormatUint(uint64(deviceId), 10)
			t.caseExpr += " WHEN " + id + " THEN " + strconv.FormatUint(uint64(seriesId), 10)
			t.deviceIds = append(t.deviceIds, id)
			logged = append(logged, c.name, fmt.Sprintf("%d → %s.%s", byColumn[c.name], source, c.field))
		}
		slog.Info("Series migration: device to migrate", append([]any{"device_id", deviceId, "rows", byColumn[""]}, logged...)...)
	}

	statements := map[string]string{}
	for _, c := range columns {
		t := targets[c.name]
		if t == nil {
			continue
		}
		sum := "NULL"
		if c.sumCol != "" {
			sum = c.sumCol
		}
		statements[c.name] = `INSERT INTO sensor_value (series_id, bucket_time, bucket_seconds, count, avg, min, max, sum) ` +
			`SELECT CASE device_id` + t.caseExpr + ` END, bucket_time, bucket_seconds, count, ` +
			c.avgCol + `, COALESCE(` + c.minCol + `, ` + c.avgCol + `), COALESCE(` + c.maxCol + `, ` + c.avgCol + `), ` + sum +
			` FROM ` + legacyTable +
			` WHERE id > ? AND id <= ? AND device_id IN (` + strings.Join(t.deviceIds, ",") + `) AND ` + c.avgCol + ` IS NOT NULL`
	}

	inserted := map[string]int64{}
	batches := 0
	for lo := fromId; lo < toId; {
		var next []uint64
		err := db.Raw(`SELECT id FROM `+legacyTable+` WHERE id > ? AND id <= ? ORDER BY id LIMIT 1 OFFSET ?`, lo, toId, migrateBatch-1).
			Scan(&next).Error
		if err != nil {
			return err
		}
		hi := toId
		if len(next) > 0 {
			hi = next[0]
		}

		err = db.Transaction(func(tx *gorm.DB) error {
			for _, c := range columns {
				statement, ok := statements[c.name]
				if !ok {
					continue
				}
				res := tx.Exec(statement, lo, hi)
				if res.Error != nil {
					return fmt.Errorf("%s: %w", c.name, res.Error)
				}
				inserted[c.name] += res.RowsAffected
			}
			return setState(tx, stateKeyProgress, strconv.FormatUint(hi, 10))
		})
		if err != nil {
			return err
		}
		lo = hi
		batches++
		if batches%migrateLogBatches == 0 {
			slog.Info("Series migration: progress", "last_id", hi, "max_id", toId)
		}
	}

	var total int64
	var logged []any
	for _, c := range columns {
		total += inserted[c.name]
		logged = append(logged, c.name, inserted[c.name])
	}
	slog.Info("Series migration: copied", append([]any{"values", total, "batches", batches}, logged...)...)
	return nil
}

// verifyMigration checks every device and column of the whole table (see the top of this file).
func (init *DatabaseInitializer) verifyMigration(columns []presentColumn, devices map[uint][]string) (bool, error) {
	db := init.db
	counts, err := init.columnCounts(columns, 0)
	if err != nil {
		return false, err
	}

	ok, err := init.checkUnmappedColumns(columns)
	if err != nil {
		return false, err
	}

	type sums struct {
		Total, Matched     int64
		OldSum, NewSum     float64
		OldMin, NewMin     float64
		OldMax, NewMax     float64
		OldCount, NewCount int64
		OldSecs, NewSecs   int64
	}
	var totalOld, totalNew int64
	for deviceId, byColumn := range counts {
		deviceOk := true
		var logged []any
		for _, c := range columns {
			if byColumn[c.name] == 0 {
				continue
			}
			source := c.source(devices[deviceId])
			seriesId, err := findSeries(db, deviceId, source, c.field)
			if err != nil {
				return false, err
			}

			var s sums
			err = db.Raw(`SELECT COUNT(*) AS total, COUNT(v.id) AS matched, `+
				`COALESCE(SUM(a.`+c.avgCol+`), 0) AS old_sum, COALESCE(SUM(v.avg), 0) AS new_sum, `+
				`COALESCE(SUM(COALESCE(a.`+c.minCol+`, a.`+c.avgCol+`)), 0) AS old_min, COALESCE(SUM(v.min), 0) AS new_min, `+
				`COALESCE(SUM(COALESCE(a.`+c.maxCol+`, a.`+c.avgCol+`)), 0) AS old_max, COALESCE(SUM(v.max), 0) AS new_max, `+
				`COALESCE(SUM(a.count), 0) AS old_count, COALESCE(SUM(v.count), 0) AS new_count, `+
				`COALESCE(SUM(a.bucket_seconds), 0) AS old_secs, COALESCE(SUM(v.bucket_seconds), 0) AS new_secs `+
				`FROM `+legacyTable+` a LEFT JOIN sensor_value v ON v.series_id = ? AND v.bucket_time = a.bucket_time `+
				`WHERE a.device_id = ? AND a.`+c.avgCol+` IS NOT NULL`, seriesId, deviceId).
				Scan(&s).Error
			if err != nil {
				return false, err
			}
			good := s.Total == s.Matched && s.OldCount == s.NewCount && s.OldSecs == s.NewSecs &&
				sameSum(s.OldSum, s.NewSum) && sameSum(s.OldMin, s.NewMin) && sameSum(s.OldMax, s.NewMax)
			totalOld += s.Total
			totalNew += s.Matched

			if c.sumCol != "" {
				var e sums
				err = db.Raw(`SELECT COUNT(*) AS total, COUNT(v.sum) AS matched, `+
					`COALESCE(SUM(a.`+c.sumCol+`), 0) AS old_sum, COALESCE(SUM(v.sum), 0) AS new_sum `+
					`FROM `+legacyTable+` a LEFT JOIN sensor_value v ON v.series_id = ? AND v.bucket_time = a.bucket_time `+
					`WHERE a.device_id = ? AND a.`+c.sumCol+` IS NOT NULL`, seriesId, deviceId).
					Scan(&e).Error
				if err != nil {
					return false, err
				}
				if e.Total != e.Matched || !sameSum(e.OldSum, e.NewSum) {
					good = false
					slog.Error("Series migration verification: energy mismatch (e.g. power_consumed without power_avg)",
						"device_id", deviceId, "old_values", e.Total, "migrated", e.Matched, "old_wh", e.OldSum, "new_wh", e.NewSum)
				}
			}

			if !good {
				deviceOk = false
				slog.Error("Series migration verification: mismatch", "device_id", deviceId, "column", c.name,
					"series_id", seriesId, "old_values", s.Total, "migrated", s.Matched, "old_sum", s.OldSum, "new_sum", s.NewSum,
					"old_count", s.OldCount, "new_count", s.NewCount, "old_bucket_secs", s.OldSecs, "new_bucket_secs", s.NewSecs)
			}
			logged = append(logged, c.name, fmt.Sprintf("%d/%d", s.Matched, s.Total))
		}
		if !deviceOk {
			ok = false
		}
		slog.Info("Series migration verification", append([]any{"device_id", deviceId, "ok", deviceOk}, logged...)...)
	}
	slog.Info("Series migration verification total", "ok", ok, "old_values", totalOld, "migrated", totalNew)
	return ok, nil
}

// checkUnmappedColumns fails when a column of the table that no series takes (an unexpected
// name, a column added later) holds any value, or a min/max is set without its average:
// dropping the table would lose it.
func (init *DatabaseInitializer) checkUnmappedColumns(columns []presentColumn) (bool, error) {
	mapped := map[string]bool{"id": true, "device_id": true, "bucket_time": true, "bucket_seconds": true, "count": true}
	for _, c := range columns {
		mapped[c.avgCol], mapped[c.minCol], mapped[c.maxCol] = true, true, true
		if c.sumCol != "" {
			mapped[c.sumCol] = true
		}
	}
	types, err := init.db.Migrator().ColumnTypes(legacyTable)
	if err != nil {
		return false, err
	}
	ok := true

	// min/max are only copied with their average
	for _, c := range columns {
		if c.minCol == c.avgCol && c.maxCol == c.avgCol {
			continue
		}
		var n int64
		err := init.db.Raw(`SELECT COUNT(*) FROM ` + legacyTable + ` WHERE ` + c.avgCol + ` IS NULL AND (` +
			c.minCol + ` IS NOT NULL OR ` + c.maxCol + ` IS NOT NULL)`).Scan(&n).Error
		if err != nil {
			return false, err
		}
		if n > 0 {
			ok = false
			slog.Error("Series migration verification: min/max without an average can't be migrated", "column", c.name, "rows", n)
		}
	}

	for _, t := range types {
		if mapped[t.Name()] {
			continue
		}
		var n int64
		if err := init.db.Raw(`SELECT COUNT("` + t.Name() + `") FROM ` + legacyTable).Scan(&n).Error; err != nil {
			return false, err
		}
		if n > 0 {
			ok = false
			slog.Error("Series migration verification: sensor_aggregate column not migrated has values", "column", t.Name(), "values", n)
		}
	}
	return ok, nil
}

func sameSum(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9*max(1, math.Abs(a))
}

// backup writes a copy of the database next to it with VACUUM INTO (as .tmp, renamed when
// complete) and returns its path.
func (init *DatabaseInitializer) backup() (string, error) {
	path, ok := sqliteFilePath(init.dsn)
	if !ok {
		return "", fmt.Errorf("STORAGE_DSN %q is not a plain database file path", init.dsn)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("database file: %w", err)
	}
	size := uint64(info.Size())
	if wal, err := os.Stat(path + "-wal"); err == nil {
		size += uint64(wal.Size())
	}
	free, err := freeSpace(filepath.Dir(path))
	if err != nil {
		return "", fmt.Errorf("free space: %w", err)
	}
	if free < backupSpaceFactor*size {
		slog.Error("Series migration: not enough free disk space for the backup and the migration",
			"db_bytes", size, "free_bytes", free, "needed_bytes", backupSpaceFactor*size)
		return "", fmt.Errorf("not enough free disk space: %d bytes free, %d needed (%d× the database)", free, backupSpaceFactor*size, backupSpaceFactor)
	}

	target := fmt.Sprintf("%s.pre-series-%s.bak", path, time.Now().Format("20060102-150405"))
	tmp := target + ".tmp"
	_ = os.Remove(tmp)
	slog.Info("Series migration: writing backup", "path", target, "db_bytes", size, "free_bytes", free)
	if err := init.db.Exec("VACUUM INTO ?", tmp).Error; err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("VACUUM INTO %s: %w", tmp, err)
	}
	written, err := os.Stat(tmp)
	if err != nil || written.Size() == 0 {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("backup %s missing or empty after VACUUM INTO: %v", tmp, err)
	}
	if err := init.checkBackupRows(tmp); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("rename backup: %w", err)
	}
	slog.Info("Series migration: backup written", "path", target, "bytes", written.Size())
	return target, nil
}

// checkBackupRows opens the backup on its own connection and compares its sensor_aggregate row
// count with the live table's.
func (init *DatabaseInitializer) checkBackupRows(path string) error {
	var live int64
	if err := init.db.Table(legacyTable).Count(&live).Error; err != nil {
		return fmt.Errorf("count %s: %w", legacyTable, err)
	}
	backup, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return fmt.Errorf("open backup %s: %w", path, err)
	}
	if sqlDb, err := backup.DB(); err == nil {
		defer sqlDb.Close()
	}
	var copied int64
	if err := backup.Table(legacyTable).Count(&copied).Error; err != nil {
		return fmt.Errorf("count %s in backup %s: %w", legacyTable, path, err)
	}
	if copied != live {
		return fmt.Errorf("backup %s has %d %s rows, the database %d", path, copied, legacyTable, live)
	}
	slog.Info("Series migration: backup checked", "rows", copied)
	return nil
}

// freeSpace returns the bytes available to this process on the file system of dir.
func freeSpace(dir string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}

// sqliteFilePath returns the file of a SQLite DSN ("data.db", "file:data.db?_busy_timeout=5000");
// false for an in-memory database.
func sqliteFilePath(dsn string) (string, bool) {
	path := strings.TrimPrefix(dsn, "file:")
	query := ""
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path, query = path[:i], path[i+1:]
	}
	if path == "" || path == ":memory:" || strings.Contains(query, "mode=memory") {
		return "", false
	}
	return path, true
}

func ensureStateTable(db *gorm.DB) error {
	return db.Exec(`CREATE TABLE IF NOT EXISTS migration_state (name TEXT PRIMARY KEY, value TEXT NOT NULL)`).Error
}

func getState(db *gorm.DB, name string) (string, bool, error) {
	var values []string
	if err := db.Raw(`SELECT CAST(value AS TEXT) FROM migration_state WHERE name = ?`, name).Scan(&values).Error; err != nil {
		return "", false, err
	}
	if len(values) == 0 {
		return "", false, nil
	}
	return values[0], true, nil
}

func setState(db *gorm.DB, name, value string) error {
	return db.Exec(`INSERT INTO migration_state (name, value) VALUES (?, ?) ON CONFLICT (name) DO UPDATE SET value = excluded.value`,
		name, value).Error
}

func deleteState(db *gorm.DB, name string) error {
	return db.Exec(`DELETE FROM migration_state WHERE name = ?`, name).Error
}
