package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"smart-home/model"
	"strconv"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

type DeviceUpsertRequest struct {
	Name           string `json:"name"`
	Topic          string `json:"topic"`
	Enabled        bool   `json:"enabled"`
	SensorType     string `json:"sensorType"`
	SupportsToggle bool   `json:"supportsToggle"`
}

func (c *DevicesController) Create(w http.ResponseWriter, r *http.Request) {
	var req DeviceUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	device := &model.Device{
		Name:       req.Name,
		Topic:      req.Topic,
		SensorType: req.SensorType,
	}
	err := c.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(device).Error; err != nil {
			return err
		}

		// insert replaces false with the column default (true), update doesn't
		device.Enabled = req.Enabled
		device.SupportsToggle = req.SupportsToggle && req.SensorType != model.SensorTypeIR
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
	json.NewEncoder(w).Encode(NewDeviceResponse(c.states.GetById(device.ID)))
}

func (c *DevicesController) Update(w http.ResponseWriter, r *http.Request) {
	deviceId, err := strconv.Atoi(chi.URLParam(r, "deviceId"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	var req DeviceUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	state := c.states.GetById(uint(deviceId))
	if state == nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	device := state.Device
	device.Name = req.Name
	device.Topic = req.Topic
	device.Enabled = req.Enabled
	device.SensorType = req.SensorType
	device.SupportsToggle = req.SupportsToggle && req.SensorType != model.SensorTypeIR

	if err := c.db.WithContext(r.Context()).Save(device).Error; err != nil {
		slog.Error("[device][update] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	c.syncSubscription(device)
	slog.Info("[device][update] success", "id", deviceId)
	json.NewEncoder(w).Encode(NewDeviceResponse(state))
}

func (c *DevicesController) Delete(w http.ResponseWriter, r *http.Request) {
	deviceId, err := strconv.Atoi(chi.URLParam(r, "deviceId"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if c.states.GetById(uint(deviceId)) == nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	if err := c.db.WithContext(r.Context()).Delete(&model.Device{}, uint(deviceId)).Error; err != nil {
		slog.Error("[device][delete] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	c.db.WithContext(r.Context()).Where("device_id = ?", uint(deviceId)).Delete(&model.IrSignal{})
	c.db.WithContext(r.Context()).Where("rule_id IN (?)", c.db.Model(&model.IrRule{}).Select("id").Where("device_id = ?", uint(deviceId))).Delete(&model.IrRuleAction{})
	c.db.WithContext(r.Context()).Where("device_id = ?", uint(deviceId)).Delete(&model.IrRule{})
	c.db.WithContext(r.Context()).Where("device_id = ?", uint(deviceId)).Delete(&model.NotifyRule{})
	if err := c.notifier.Reload(); err != nil {
		slog.Error("[device][delete] notifier reload error", "err", err)
	}
	c.consumer.Remove(uint(deviceId))
	c.states.Delete(uint(deviceId))
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
	deviceId, err := strconv.Atoi(chi.URLParam(r, "deviceId"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if c.states.GetById(uint(deviceId)) == nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	if err := c.db.WithContext(r.Context()).Where("device_id = ?", uint(deviceId)).Delete(&model.SensorAggregate{}).Error; err != nil && err != gorm.ErrRecordNotFound {
		slog.Error("[device][wipe-sensor-data] aggregate error", "err", err, "id", deviceId)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	if err := c.db.WithContext(r.Context()).Where("device_id = ?", uint(deviceId)).Delete(&model.SensorHistory{}).Error; err != nil && err != gorm.ErrRecordNotFound {
		slog.Error("[device][wipe-sensor-data] history error", "err", err, "id", deviceId)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	c.storage.ClearBuffer(uint(deviceId))
	slog.Info("[device][wipe-sensor-data] success", "id", deviceId)
	w.WriteHeader(http.StatusNoContent)
}
