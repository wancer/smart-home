package web

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"smart-home/internal"
	"smart-home/mqtt"
	"strconv"
)

type DeviceControlController struct {
	pub    *mqtt.Publisher
	states *internal.StateStorage
}

func NewDeviceControlController(
	pub *mqtt.Publisher,
	states *internal.StateStorage,
) *DeviceControlController {
	return &DeviceControlController{
		pub:    pub,
		states: states,
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

	device := c.states.GetById(parsed.DeviceId)
	if device == nil {
		slog.Error("[device][control] error", "err", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	err = c.handle(&parsed, device)
	if err != nil {
		slog.Error("[device][control] error", "err", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	slog.Info("[device][control] success", "deviceId", parsed.DeviceId, "parameter", parsed.Parameter, "value", parsed.Value)
	w.WriteHeader(http.StatusOK)
}

func (c *DeviceControlController) handle(r *DeviceControlRequest, d *internal.DeviceState) error {
	switch r.Parameter {
	case "on-off":
		c.pub.OnOff(d.Device, r.Value == "ON")
	case "voltage":
		volts, err := strconv.Atoi(r.Value)
		if err != nil {
			return err
		}
		c.pub.SetVoltage(d.Device, volts)
	case "power":
		power, err := strconv.Atoi(r.Value)
		if err != nil {
			return err
		}
		volts := d.Voltage
		if volts == nil {
			return fmt.Errorf("Device don't have volts")
		}

		c.pub.SetPower(d.Device, *volts, power)
	default:
		return fmt.Errorf("unknown type: %s", r.Parameter)
	}

	return nil
}
