package web

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"smart-home/internal"
	"smart-home/mqtt"
)

type DeviceControlController struct {
	pub       *mqtt.Publisher
	deviceMap *internal.DeviceMap
}

func NewDeviceControlController(
	pub *mqtt.Publisher,
	deviceMap *internal.DeviceMap,
) *DeviceControlController {
	return &DeviceControlController{
		pub:       pub,
		deviceMap: deviceMap,
	}
}

type DeviceControlRequest struct {
	DeviceId  uint   `json:"deviceId"`
	Parameter string `json:"parameter"`
	Value     string `json:"value"`
}

func (c *DeviceControlController) Do(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("[device][control] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	var parsed DeviceControlRequest
	err = json.Unmarshal(body, &parsed)
	if err != nil {
		slog.Error("[device][control] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if parsed.Parameter != "power" {
		slog.Error("[device][control] error", "err", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	device := c.deviceMap.GeyById(parsed.DeviceId)
	if device == nil {
		slog.Error("[device][control] error", "err", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	c.pub.SendSetPower(device, parsed.Value == "ON")

	w.WriteHeader(http.StatusOK)
}
