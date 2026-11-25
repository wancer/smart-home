package web

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"smart-home/model"
	"time"

	"gorm.io/gorm"
)

type DeviceUpsertRequest struct {
	Name           string `json:"name"`
	Topic          string `json:"topic"`
	Enabled        bool   `json:"enabled"`
	OfflineTimeout uint   `json:"offlineTimeout"` // seconds, 0 = auto
	// Platform is model.PlatformTasmota or model.PlatformEsphome; omitted = tasmota on create,
	// unchanged on update.
	Platform *string `json:"platform"`
	// StorageTiers is the sensor data retention: [] = the default; omitted = the default
	// on create, unchanged on update.
	StorageTiers *[]model.StorageTier `json:"storageTiers"`
}

// decodeDeviceRequest reads and validates a create/update body, answering 400 itself.
func decodeDeviceRequest(w http.ResponseWriter, r *http.Request) (*DeviceUpsertRequest, bool) {
	var req DeviceUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return nil, false
	}
	if req.Platform != nil && !model.ValidPlatform(*req.Platform) {
		http.Error(w, "unknown platform", http.StatusBadRequest)
		return nil, false
	}
	if req.StorageTiers != nil {
		if err := model.ValidateStorageTiers(*req.StorageTiers); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return nil, false
		}
	}
	return &req, true
}

func (c *DevicesController) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeDeviceRequest(w, r)
	if !ok {
		return
	}

	device := &model.Device{
		Name:       req.Name,
		Topic:      req.Topic,
		SensorType: model.SensorTypeOther, // detected once the device replies
		Platform:   model.PlatformTasmota,
	}
	if req.Platform != nil {
		device.Platform = *req.Platform
	}
	err := c.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(device).Error; err != nil {
			return err
		}

		// insert replaces false with the column default (true), update doesn't
		device.Enabled = req.Enabled
		device.OfflineTimeout = req.OfflineTimeout
		if req.StorageTiers != nil {
			device.StorageTiers = *req.StorageTiers
		}
		return tx.Save(device).Error
	})
	if err != nil {
		slog.Error("[device][create] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	c.states.Add(device)
	c.syncSubscription(device)
	slog.Info("[device][create] success", "id", device.ID)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(NewDeviceResponse(c.states.GetById(device.ID), c.storage))
}

func (c *DevicesController) Update(w http.ResponseWriter, r *http.Request) {
	state := deviceState(w, r, c.states)
	if state == nil {
		return
	}

	req, ok := decodeDeviceRequest(w, r)
	if !ok {
		return
	}

	// Save writes every column; the profile ones may change from MQTT, so they're left out.
	device := state.Device
	device.Name = req.Name
	device.Topic = req.Topic
	device.Enabled = req.Enabled
	device.OfflineTimeout = req.OfflineTimeout
	if req.StorageTiers != nil {
		device.StorageTiers = *req.StorageTiers
	}
	columns := []any{"Topic", "Enabled", "Platform", "OfflineTimeout", "StorageTiers"}
	if req.Platform != nil && *req.Platform != device.Platform {
		device.Platform = *req.Platform
		resetFirmwareProfile(device)
		columns = append(columns, "Template", "Chip", "FirmwareVersion", "FirmwareBuiltAt", "FirmwareCore", "FirmwareSdk", "SensorType")
	}

	err := c.db.WithContext(r.Context()).Model(device).
		Select("Name", columns...).
		Updates(device).Error
	if err != nil {
		slog.Error("[device][update] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	c.syncSubscription(device)
	slog.Info("[device][update] success", "id", device.ID)
	json.NewEncoder(w).Encode(NewDeviceResponse(state, c.storage))
}

// resetFirmwareProfile drops what was read from the old platform's firmware (template, chip,
// version) on a platform change. Seen sensors stay; the sensor type is re-detected from
// telemetry (an IR remote or a light, known from its template, starts over as "other").
func resetFirmwareProfile(d *model.Device) {
	d.Template = nil
	d.Chip = ""
	d.FirmwareVersion = ""
	d.FirmwareBuiltAt = nil
	d.FirmwareCore = ""
	d.FirmwareSdk = ""
	if d.SensorType == model.SensorTypeIR || d.SensorType == model.SensorTypeLight {
		d.SensorType = model.SensorTypeOther
	}
}

// UpdateDashboard sets what the dashboard shows for the device (model.DashboardView:
// {chart: seriesId|null, noChart, values: [seriesId]|null}).
func (c *DevicesController) UpdateDashboard(w http.ResponseWriter, r *http.Request) {
	state := deviceState(w, r, c.states)
	if state == nil {
		return
	}
	deviceId := state.Device.ID

	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	var view model.DashboardView
	if err := json.Unmarshal(body, &view); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	owned := func(id uint) bool {
		s, ok := c.storage.Series(id)
		return ok && s.DeviceId == deviceId
	}
	if err := model.ValidateDashboardView(view, owned); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	device := state.Device
	device.Dashboard = &view
	if view.IsAuto() {
		device.Dashboard = nil
	}
	err = c.db.WithContext(r.Context()).Model(device).Select("Dashboard").Updates(device).Error
	if err != nil {
		slog.Error("[device][dashboard] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	slog.Info("[device][dashboard] success", "id", device.ID)
	json.NewEncoder(w).Encode(NewDeviceResponse(state, c.storage))
}

func (c *DevicesController) Delete(w http.ResponseWriter, r *http.Request) {
	state := deviceState(w, r, c.states)
	if state == nil {
		return
	}
	deviceId := state.Device.ID

	// unsubscribe first, so telemetry can't recreate series of the deleted device
	c.consumer.Remove(deviceId)
	// the device row, its series and their values in one transaction
	err := c.storage.DeleteDevice(c.db.WithContext(r.Context()), deviceId, func(tx *gorm.DB) error {
		return tx.Delete(&model.Device{}, deviceId).Error
	})
	if err != nil {
		slog.Error("[device][delete] error", "err", err)
		c.consumer.Sync(state.Device) // still there: subscribe again
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	c.db.WithContext(r.Context()).Where("device_id = ?", deviceId).Delete(&model.IrSignal{})
	c.db.WithContext(r.Context()).Where("rule_id IN (?)", c.db.Model(&model.IrRule{}).Select("id").Where("device_id = ?", deviceId)).Delete(&model.IrRuleAction{})
	c.db.WithContext(r.Context()).Where("device_id = ?", deviceId).Delete(&model.IrRule{})
	c.db.WithContext(r.Context()).Where("device_id = ?", deviceId).Delete(&model.NotifyRule{})
	if err := c.notifier.Reload(); err != nil {
		slog.Error("[device][delete] notifier reload error", "err", err)
	}
	c.states.Delete(deviceId)
	slog.Info("[device][delete] success", "id", deviceId)
	w.WriteHeader(http.StatusNoContent)
}

// syncSubscription applies device changes to MQTT. A freshly subscribed device
// is polled, otherwise its state stays empty until the next TelePeriod.
func (c *DevicesController) syncSubscription(device *model.Device) {
	if c.consumer.Sync(device) {
		c.pub.PublishStates(device)
	}
}

func (c *DevicesController) WipeSensorData(w http.ResponseWriter, r *http.Request) {
	state := deviceState(w, r, c.states)
	if state == nil {
		return
	}
	deviceId := state.Device.ID

	// Series, values and daily history in one transaction.
	err := c.storage.DeleteDevice(c.db.WithContext(r.Context()), deviceId, func(tx *gorm.DB) error {
		return tx.Where("device_id = ?", deviceId).Delete(&model.SensorHistory{}).Error
	})
	if err != nil {
		slog.Error("[device][wipe-sensor-data] error", "err", err, "id", deviceId)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	slog.Info("[device][wipe-sensor-data] success", "id", deviceId)
	w.WriteHeader(http.StatusNoContent)
}

// RefreshProfile re-reads the template, chip/firmware and sensors from the device and
// re-detects the sensor type (which may then also go down, e.g. to "other"). 409 if offline.
func (c *DevicesController) RefreshProfile(w http.ResponseWriter, r *http.Request) {
	state := tasmotaDevice(w, r, c.states)
	if state == nil {
		return
	}
	if !state.Online {
		http.Error(w, http.StatusText(http.StatusConflict), http.StatusConflict)
		return
	}

	state.ProfileRefresh = true // the STATUS10 reply detects the type in any direction
	c.pub.GetTemplate(state.Device)
	c.pub.GetFirmware(state.Device)
	c.pub.GetSensors(state.Device)
	time.Sleep(3 * time.Second)
	state.ProfileRefresh = false

	slog.Info("[device][profile-refresh] success", "id", state.Device.ID, "sensorType", state.Device.SensorType)
	json.NewEncoder(w).Encode(NewDeviceResponse(state, c.storage))
}
