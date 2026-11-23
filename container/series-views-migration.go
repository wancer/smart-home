package container

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"smart-home/internal"
	"smart-home/model"
)

// One-off migrations of what referred to metric keys before sensor series: the dashboard view
// (device.dashboard) and the notification rules (notify_rule.metric). Both are idempotent and
// run on every start after the series migration; once everything refers to series they do
// nothing.

// deviceSeriesPreferred returns the device's series per kind, preferred one first.
func (init *DatabaseInitializer) deviceSeriesPreferred(deviceId uint) (map[string][]uint, error) {
	var series []model.SensorSeries
	if err := init.db.Where("device_id = ?", deviceId).Find(&series).Error; err != nil {
		return nil, err
	}
	return internal.PreferredSeriesOf(series), nil
}

// isLegacyDashboard reports whether a stored dashboard view uses metric keys (a string chart
// or string values) instead of series IDs.
func isLegacyDashboard(raw []byte) (bool, error) {
	var probe struct {
		Chart  json.RawMessage   `json:"chart"`
		Values []json.RawMessage `json:"values"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false, err
	}
	legacy := len(probe.Chart) > 0 && probe.Chart[0] == '"'
	for _, v := range probe.Values {
		legacy = legacy || (len(v) > 0 && v[0] == '"')
	}
	return legacy, nil
}

// migrateDashboardViews rewrites device.dashboard views by metric key ({chart: "power", values:
// ["power", "temperature"]}) as views by series: each key becomes the device's preferred series
// of its kind; keys without such a series are dropped (logged). It must run before the devices
// are loaded, which can't decode the legacy form. An unreadable view is reset to auto.
func (init *DatabaseInitializer) migrateDashboardViews() error {
	var rows []struct {
		Id        uint
		Dashboard sql.NullString
	}
	err := init.db.Raw("SELECT id, dashboard FROM device WHERE dashboard IS NOT NULL AND dashboard <> '' AND dashboard <> 'null'").
		Scan(&rows).Error
	if err != nil {
		return err
	}
	for _, row := range rows {
		raw := []byte(row.Dashboard.String)
		legacy, err := isLegacyDashboard(raw)
		if err != nil {
			slog.Warn("Unreadable dashboard view, reset to auto", "device_id", row.Id, "err", err)
			if err := init.db.Exec("UPDATE device SET dashboard = NULL WHERE id = ?", row.Id).Error; err != nil {
				return err
			}
			continue
		}
		if !legacy {
			continue
		}

		var old model.LegacyDashboardView
		if err := json.Unmarshal(raw, &old); err != nil {
			return fmt.Errorf("dashboard view of device %d: %w", row.Id, err)
		}
		preferred, err := init.deviceSeriesPreferred(row.Id)
		if err != nil {
			return err
		}
		view, err := old.FromLegacy(preferred)
		if err != nil {
			slog.Warn("Dashboard view with an unknown metric, reset to auto", "device_id", row.Id, "err", err)
			view = model.DashboardView{}
		}

		var value any // NULL = auto
		if !view.IsAuto() {
			encoded, err := json.Marshal(view)
			if err != nil {
				return err
			}
			value = string(encoded)
		}
		if err := init.db.Exec("UPDATE device SET dashboard = ? WHERE id = ?", value, row.Id).Error; err != nil {
			return err
		}
		slog.Info("Migrated dashboard view to series", "device_id", row.Id, "from", string(raw), "to", value)
		if len(view.Values) < len(old.Values) || (old.Chart != model.LegacyChartAuto && old.Chart != model.LegacyChartNone && view.Chart == nil) {
			slog.Warn("Dashboard view: some metrics have no series on the device and were dropped", "device_id", row.Id)
		}
	}
	return nil
}

// migrateNotifyRules sets the series of threshold rules made before series (notify_rule.metric,
// which equals a series kind: power, temperature, gas_ratio, ...): the device's preferred series
// of that kind, clearing metric. A rule whose device has no such series keeps its metric and is
// disabled (logged); a later start maps it once the series exists (it stays disabled).
func (init *DatabaseInitializer) migrateNotifyRules() error {
	var rules []model.NotifyRule
	err := init.db.Where("series_id IS NULL AND metric IS NOT NULL AND metric <> '' AND kind IN ?",
		[]string{model.NotifyAbove, model.NotifyBelow}).Find(&rules).Error
	if err != nil {
		return err
	}
	for _, rule := range rules {
		preferred, err := init.deviceSeriesPreferred(rule.DeviceId)
		if err != nil {
			return err
		}
		if ids := preferred[rule.Metric]; len(ids) > 0 {
			err := init.db.Model(&model.NotifyRule{}).Where("id = ?", rule.ID).
				Updates(map[string]any{"series_id": ids[0], "metric": ""}).Error
			if err != nil {
				return err
			}
			slog.Info("Migrated notification rule to series", "rule_id", rule.ID, "device_id", rule.DeviceId, "metric", rule.Metric, "series_id", ids[0])
			continue
		}
		if rule.Enabled {
			if err := init.db.Model(&model.NotifyRule{}).Where("id = ?", rule.ID).Update("enabled", false).Error; err != nil {
				return err
			}
		}
		slog.Warn("Notification rule: the device has no series for its metric, rule disabled",
			"rule_id", rule.ID, "device_id", rule.DeviceId, "metric", rule.Metric)
	}
	return nil
}
