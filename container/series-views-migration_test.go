package container

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"smart-home/model"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// viewsDb has two devices with series and legacy dashboard views / notification rules.
func viewsDb(t *testing.T) (*gorm.DB, map[string]uint) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "views.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Device{}, &model.SensorSeries{}, &model.NotifyRule{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.Device{{ID: 1, Name: "climate", Topic: "climate"}, {ID: 2, Name: "plug", Topic: "plug"}, {ID: 3, Name: "new", Topic: "new"}}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	series := []model.SensorSeries{
		{DeviceId: 1, Source: "SCD40", Field: "Temperature", Kind: model.KindTemperature},
		{DeviceId: 1, Source: "SHT4X", Field: "Temperature", Kind: model.KindTemperature},
		{DeviceId: 1, Source: "SCD40", Field: "CarbonDioxide", Kind: model.KindCo2},
		{DeviceId: 1, Source: "SHT4X", Field: "Humidity", Kind: model.KindHumidity},
		{DeviceId: 2, Source: "ENERGY", Field: "Power", Kind: model.KindPower},
	}
	ids := map[string]uint{}
	for i := range series {
		series[i].FirstSeen, series[i].LastSeen = now, now
		if err := db.Create(&series[i]).Error; err != nil {
			t.Fatal(err)
		}
		ids[series[i].Source+"."+series[i].Field] = series[i].Id
	}
	exec := func(q string, args ...any) {
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE device SET dashboard = ? WHERE id = 1`, `{"chart":"temperature","values":["co2","temperature","pressure"]}`)
	exec(`UPDATE device SET dashboard = ? WHERE id = 2`, `{"chart":"none","values":null}`)
	exec(`UPDATE device SET dashboard = ? WHERE id = 3`, `{"chart":"","values":null}`)
	exec(`INSERT INTO notify_rule (id, device_id, kind, metric, threshold, duration_sec, enabled) VALUES
		(1, 1, 'above', 'co2', 1000, 0, 1),
		(2, 1, 'below', 'humidity', 30, 0, 1),
		(3, 2, 'below', 'temperature', 5, 0, 1),
		(4, 2, 'offline', '', 0, 300, 1)`)
	return db, ids
}

func TestMigrateDashboardViews(t *testing.T) {
	db, ids := viewsDb(t)
	init := NewDatabaseInitializer(db, "")
	for range 2 { // idempotent
		if err := init.migrateDashboardViews(); err != nil {
			t.Fatal(err)
		}
	}

	var devices []model.Device
	if err := db.Order("id").Find(&devices).Error; err != nil {
		t.Fatalf("devices must load after the migration: %v", err)
	}
	// the T-H sensor's temperature is preferred over the CO₂ sensor's; pressure has no series
	got, _ := json.Marshal(devices[0].Dashboard)
	want, _ := json.Marshal(model.DashboardView{
		Chart:  new(ids["SHT4X.Temperature"]),
		Values: []uint{ids["SCD40.CarbonDioxide"], ids["SHT4X.Temperature"]},
	})
	if string(got) != string(want) {
		t.Errorf("device 1: %s, want %s", got, want)
	}
	if v := devices[1].Dashboard; v == nil || !v.NoChart || v.Chart != nil || v.Values != nil {
		t.Errorf("device 2: %+v, want no chart", v)
	}
	if devices[2].Dashboard != nil {
		t.Errorf("device 3: %+v, want NULL (auto)", devices[2].Dashboard)
	}
}

func TestMigrateNotifyRules(t *testing.T) {
	db, ids := viewsDb(t)
	init := NewDatabaseInitializer(db, "")
	for range 2 {
		if err := init.migrateNotifyRules(); err != nil {
			t.Fatal(err)
		}
	}

	var rules []model.NotifyRule
	if err := db.Order("id").Find(&rules).Error; err != nil {
		t.Fatal(err)
	}
	check := func(r model.NotifyRule, series uint, metric string, enabled bool) {
		t.Helper()
		var got uint
		if r.SeriesId != nil {
			got = *r.SeriesId
		}
		if got != series || r.Metric != metric || r.Enabled != enabled {
			t.Errorf("rule %d: series %d metric %q enabled %v, want %d %q %v", r.ID, got, r.Metric, r.Enabled, series, metric, enabled)
		}
	}
	check(rules[0], ids["SCD40.CarbonDioxide"], "", true)
	check(rules[1], ids["SHT4X.Humidity"], "", true)
	check(rules[2], 0, "temperature", false) // no temperature series on the plug: disabled, metric kept
	check(rules[3], 0, "", true)             // offline rule untouched

	// the series appears later: mapped on the next start, stays disabled
	now := time.Now()
	s := model.SensorSeries{DeviceId: 2, Source: "DS18B20", Field: "Temperature", Kind: model.KindTemperature, FirstSeen: now, LastSeen: now}
	if err := db.Create(&s).Error; err != nil {
		t.Fatal(err)
	}
	if err := init.migrateNotifyRules(); err != nil {
		t.Fatal(err)
	}
	var rule model.NotifyRule
	db.First(&rule, 3)
	check(rule, s.Id, "", false)
}

func TestMigrateDashboardViewsUnreadable(t *testing.T) {
	db, _ := viewsDb(t)
	if err := db.Exec(`UPDATE device SET dashboard = 'garbage' WHERE id = 1`).Error; err != nil {
		t.Fatal(err)
	}
	if err := NewDatabaseInitializer(db, "").migrateDashboardViews(); err != nil {
		t.Fatal(err)
	}
	var value sql.NullString
	db.Raw("SELECT dashboard FROM device WHERE id = 1").Scan(&value)
	if value.Valid {
		t.Errorf("unreadable view kept: %q", value.String)
	}
}
