package container

import (
	"path/filepath"
	"smart-home/model"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var legacyAt = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

// legacyDb creates a database file with devices and sensor_aggregate rows as the old version
// wrote them: an energy plug with chip temperature (one row without min/max, one with an old
// +03:00 offset, one raw row), a climate board (BME280 + SCD40), a light board (BH1750 +
// TCS34725) and rows of a deleted device. 7 rows, 14 values.
func legacyDb(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Device{}, &model.SensorAggregate{}, &model.SensorSeries{}, &model.SensorValue{}); err != nil {
		t.Fatal(err)
	}
	devices := []model.Device{
		{ID: 1, Name: "plug", Topic: "plug", Sensors: []string{"ENERGY", "ESP32"}},
		{ID: 2, Name: "climate", Topic: "climate", Sensors: []string{"BME280", "SCD40"}},
		{ID: 3, Name: "light", Topic: "light", Sensors: []string{"BH1750", "TCS34725"}},
	}
	if err := db.Create(&devices).Error; err != nil {
		t.Fatal(err)
	}

	at := legacyAt
	f := func(v float32) *float32 { return &v }
	rows := []model.SensorAggregate{
		{DeviceId: 1, BucketTime: at, BucketSeconds: 300, Count: 5,
			PowerAvg: f(100), PowerMin: f(90), PowerMax: f(110), PowerConsumed: f(8.3),
			VoltageAvg: f(230), VoltageMin: f(229), VoltageMax: f(231), ChipTemperatureAvg: f(45)},
		{DeviceId: 1, BucketTime: at.Add(5 * time.Minute), BucketSeconds: 300, Count: 5,
			PowerAvg: f(120), PowerConsumed: f(10)}, // no min/max (stored before they were tracked)
		{DeviceId: 1, BucketTime: time.Date(2025, 3, 1, 15, 0, 0, 0, time.FixedZone("", 3*3600)), BucketSeconds: 3600, Count: 60,
			PowerAvg: f(50), PowerMin: f(0), PowerMax: f(200), PowerConsumed: f(50)},
		{DeviceId: 2, BucketTime: at, BucketSeconds: 300, Count: 10,
			CO2Avg: f(600), CO2Min: f(580), CO2Max: f(620), TemperatureAvg: f(21.5), TemperatureMin: f(21), TemperatureMax: f(22),
			HumidityAvg: f(40), HumidityMin: f(39), HumidityMax: f(41), DewPointAvg: f(7.5), PressureAvg: f(1013), PressureMin: f(1012), PressureMax: f(1014)},
		{DeviceId: 3, BucketTime: at, BucketSeconds: 300, Count: 1,
			IlluminanceAvg: f(200), IlluminanceMin: f(200), IlluminanceMax: f(200), ColorTempAvg: f(3500), ColorTempMin: f(3500), ColorTempMax: f(3500)},
		{DeviceId: 99, BucketTime: at, BucketSeconds: 300, Count: 2, TemperatureAvg: f(19), TemperatureMin: f(18), TemperatureMax: f(20)},
	}
	for i := range rows {
		if err := db.Omit(clause.Associations).Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	// a raw reading (bucket_seconds 0; GORM would write the column default 300 for a zero value)
	err = db.Exec(`INSERT INTO sensor_aggregate (device_id, bucket_time, bucket_seconds, count, power_avg, power_min, power_max, power_consumed)
		VALUES (1, '2026-11-20 12:10:01+00:00', 0, 1, 77, 77, 77, 0.1)`).Error
	if err != nil {
		t.Fatal(err)
	}
	return db, path
}

// runMigration does what DatabaseInitializer.migrate does for the series.
func runMigration(init *DatabaseInitializer) error {
	if err := init.backupForSeries(); err != nil {
		return err
	}
	return init.migrateSeries()
}

func seriesValues(t *testing.T, db *gorm.DB) map[string][]model.SensorValue {
	t.Helper()
	var series []model.SensorSeries
	if err := db.Find(&series).Error; err != nil {
		t.Fatal(err)
	}
	result := map[string][]model.SensorValue{}
	for _, s := range series {
		var values []model.SensorValue
		if err := db.Where("series_id = ?", s.Id).Order("id").Find(&values).Error; err != nil {
			t.Fatal(err)
		}
		key := string(rune('0'+s.DeviceId%10)) + ":" + s.Source + "." + s.Field + ":" + s.Kind
		result[key] = values
	}
	return result
}

func countValues(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&model.SensorValue{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func countState(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Raw(`SELECT COUNT(*) FROM migration_state WHERE name LIKE 'series.%'`).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

// checkMigrated checks the values of a finished migration of legacyDb.
func checkMigrated(t *testing.T, db *gorm.DB) {
	t.Helper()
	values := seriesValues(t, db)
	expect := map[string]int{
		"1:ENERGY.Power:power":                   4,
		"1:ENERGY.Voltage:voltage":               1,
		"1:ESP32.Temperature:chip_temperature":   1,
		"2:SCD40.CarbonDioxide:co2":              1,
		"2:BME280.Temperature:temperature":       1, // a T-H sensor before the CO₂ sensor
		"2:BME280.Humidity:humidity":             1,
		"2:BME280.DewPoint:dew_point":            1,
		"2:BME280.Pressure:pressure":             1,
		"3:BH1750.Illuminance:illuminance":       1, // the light meter before the colour sensor
		"3:TCS34725.ColorTemperature:color_temp": 1,
		"9:legacy.Temperature:temperature":       1, // deleted device 99: no sensors known
	}
	for key, n := range expect {
		if len(values[key]) != n {
			t.Errorf("%s: %d values, want %d (series: %v)", key, len(values[key]), n, keys(values))
		}
	}
	if len(values) != len(expect) {
		t.Errorf("series = %v, want %d", keys(values), len(expect))
	}
	if n := countValues(t, db); n != 14 {
		t.Errorf("values = %d, want 14", n)
	}

	power := values["1:ENERGY.Power:power"]
	byAvg := map[float64]model.SensorValue{}
	for _, p := range power {
		byAvg[p.Avg] = p
	}
	if p := byAvg[100]; p.Count != 5 || p.BucketSeconds != 300 || p.Min != 90 || p.Max != 110 || p.Sum == nil || *p.Sum < 8.29 || *p.Sum > 8.31 {
		t.Errorf("power bucket = %+v", p)
	}
	if p := byAvg[120]; p.Min != 120 || p.Max != 120 {
		t.Errorf("missing min/max must become the average: %+v", p)
	}
	if p := byAvg[77]; p.BucketSeconds != 0 || p.Count != 1 {
		t.Errorf("raw row = %+v", p)
	}
	if dew := values["2:BME280.DewPoint:dew_point"][0]; dew.Min != 7.5 || dew.Max != 7.5 {
		t.Errorf("dew point = %+v", dew)
	}

	// bucket_time text is copied as stored, including the old +03:00 offset
	var texts []string
	err := db.Raw(`SELECT CAST(bucket_time AS TEXT) FROM sensor_value WHERE bucket_seconds = 3600`).Scan(&texts).Error
	if err != nil || len(texts) != 1 || len(texts[0]) < 6 || texts[0][len(texts[0])-6:] != "+03:00" {
		t.Errorf("legacy bucket_time = %v (err %v)", texts, err)
	}
}

func TestSeriesMigrationDropsAfterBackup(t *testing.T) {
	db, path := legacyDb(t)
	init := NewDatabaseInitializer(db, path)

	if err := runMigration(init); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable(legacyTable) {
		t.Fatal("sensor_aggregate not dropped after a verified migration with backup")
	}
	checkMigrated(t, db)
	if n := countState(t, db); n != 0 {
		t.Errorf("%d migration_state rows left after the drop", n)
	}

	backups, _ := filepath.Glob(path + ".pre-series-*.bak")
	if len(backups) != 1 {
		t.Fatalf("backups = %v", backups)
	}
	if tmp, _ := filepath.Glob(path + ".pre-series-*.tmp"); len(tmp) != 0 {
		t.Errorf("temporary backup left: %v", tmp)
	}
	backup, err := gorm.Open(sqlite.Open(backups[0]), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	var n int64
	if err := backup.Table(legacyTable).Count(&n).Error; err != nil || n != 7 {
		t.Errorf("backup sensor_aggregate rows = %d (err %v), want 7", n, err)
	}

	// a later start: no table, leftover state rows are removed
	if err := setState(db, stateKeyProgress, "5"); err != nil {
		t.Fatal(err)
	}
	if err := runMigration(init); err != nil {
		t.Fatal(err)
	}
	if n := countState(t, db); n != 0 {
		t.Errorf("leftover migration_state rows not removed: %d", n)
	}
}

func TestSeriesMigrationMultiBatch(t *testing.T) {
	defer func(batch int) { migrateBatch = batch }(migrateBatch)
	migrateBatch = 2

	db, path := legacyDb(t)
	if err := runMigration(NewDatabaseInitializer(db, path)); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable(legacyTable) {
		t.Fatal("sensor_aggregate not dropped")
	}
	checkMigrated(t, db)
}

// A start interrupted after some batches resumes from the saved progress (a re-copy of a
// finished batch would conflict and fail).
func TestSeriesMigrationResumes(t *testing.T) {
	db, path := legacyDb(t)
	init := NewDatabaseInitializer(db, path)
	if err := init.backupForSeries(); err != nil {
		t.Fatal(err)
	}
	columns, err := init.presentColumns()
	if err != nil {
		t.Fatal(err)
	}
	devices, err := init.deviceSensors()
	if err != nil {
		t.Fatal(err)
	}
	if err := init.copyAggregates(columns, devices, 0, 3); err != nil { // "crash" after id 3
		t.Fatal(err)
	}

	if err := runMigration(init); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable(legacyTable) {
		t.Fatal("sensor_aggregate not dropped")
	}
	checkMigrated(t, db)
	if backups, _ := filepath.Glob(path + ".pre-series-*.bak"); len(backups) != 1 {
		t.Errorf("the backup of the interrupted start must be reused: %v", backups)
	}
}

func TestSeriesMigrationRefusesWithoutBackup(t *testing.T) {
	db, _ := legacyDb(t)
	init := NewDatabaseInitializer(db, ":memory:") // no backup possible

	if err := runMigration(init); err == nil {
		t.Fatal("expected an error without a backup")
	}
	if !db.Migrator().HasTable(legacyTable) {
		t.Fatal("sensor_aggregate dropped without a backup")
	}
	if n := countValues(t, db); n != 0 {
		t.Errorf("%d values copied without a backup", n)
	}
}

func TestSeriesMigrationConflictFails(t *testing.T) {
	db, path := legacyDb(t)
	seriesId, err := ensureSeries(db, 1, model.SourceEnergy, "Power")
	if err != nil {
		t.Fatal(err)
	}
	existing := model.SensorValue{SeriesId: seriesId, BucketTime: legacyAt, BucketSeconds: 300, Count: 1, Avg: 1, Min: 1, Max: 1}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}

	if err := runMigration(NewDatabaseInitializer(db, path)); err == nil {
		t.Fatal("expected an error on a conflicting sensor_value row")
	}
	if !db.Migrator().HasTable(legacyTable) {
		t.Fatal("sensor_aggregate dropped after a failed copy")
	}
}

func TestSeriesMigrationKeepsTableWhenVerificationFails(t *testing.T) {
	cases := map[string]func(t *testing.T, db *gorm.DB){
		"energy without power average": func(t *testing.T, db *gorm.DB) {
			consumed := float32(3)
			row := model.SensorAggregate{DeviceId: 1, BucketTime: legacyAt.Add(time.Hour), BucketSeconds: 300, Count: 1, PowerConsumed: &consumed}
			if err := db.Omit(clause.Associations).Create(&row).Error; err != nil {
				t.Fatal(err)
			}
		},
		"min without average": func(t *testing.T, db *gorm.DB) {
			low := float32(18)
			row := model.SensorAggregate{DeviceId: 2, BucketTime: legacyAt.Add(time.Hour), BucketSeconds: 300, Count: 1, TemperatureMin: &low}
			if err := db.Omit(clause.Associations).Create(&row).Error; err != nil {
				t.Fatal(err)
			}
		},
		"unmapped column with values": func(t *testing.T, db *gorm.DB) {
			if err := db.Exec(`ALTER TABLE sensor_aggregate ADD COLUMN foo_avg REAL`).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Exec(`UPDATE sensor_aggregate SET foo_avg = 1 WHERE id = 1`).Error; err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, prepare := range cases {
		t.Run(name, func(t *testing.T) {
			db, path := legacyDb(t)
			prepare(t, db)
			init := NewDatabaseInitializer(db, path)

			if err := runMigration(init); err != nil {
				t.Fatalf("a failed verification must not stop the start: %v", err)
			}
			if !db.Migrator().HasTable(legacyTable) {
				t.Fatal("sensor_aggregate dropped although verification failed")
			}
			if v, _, _ := getState(db, stateKeyVerified); v != "-1" {
				t.Errorf("verification = %q, want -1", v)
			}
			// later starts keep it too
			if err := runMigration(init); err != nil {
				t.Fatal(err)
			}
			if !db.Migrator().HasTable(legacyTable) {
				t.Fatal("sensor_aggregate dropped on a later start")
			}
		})
	}
}

func TestSqliteFilePath(t *testing.T) {
	for dsn, want := range map[string]string{
		"data/shome.db":                  "data/shome.db",
		"file:shome.db?_busy_timeout=50": "shome.db",
		":memory:":                       "",
		"file::memory:?cache=shared":     "",
		"file:x.db?mode=memory":          "",
	} {
		got, ok := sqliteFilePath(dsn)
		if got != want || ok != (want != "") {
			t.Errorf("%q: %q %v, want %q", dsn, got, ok, want)
		}
	}
}

func keys[V any](m map[string]V) []string {
	var result []string
	for k := range m {
		result = append(result, k)
	}
	return result
}
