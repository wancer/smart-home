package web

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"smart-home/config"
	"smart-home/internal"
	"smart-home/mqtt"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

type DeviceControlController struct {
	pub       *mqtt.Publisher
	states    *internal.DeviceStateStorage
	timezones *config.TimezonesConfig
}

func NewDeviceControlController(
	pub *mqtt.Publisher,
	states *internal.DeviceStateStorage,
	timezones *config.TimezonesConfig,
) *DeviceControlController {
	return &DeviceControlController{
		pub:       pub,
		states:    states,
		timezones: timezones,
	}
}

type DeviceControlRequest struct {
	DeviceId  uint   `json:"deviceId"`
	Parameter string `json:"parameter"`
	Value     string `json:"value"`
}

func (c *DeviceControlController) Get(w http.ResponseWriter, r *http.Request) {
	deviceId, err := strconv.Atoi(chi.URLParam(r, "deviceId"))
	if err != nil {
		slog.Error("[sensors][daily] error", "err", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	state := c.states.GetById(uint(deviceId))
	if state == nil {
		slog.Error("[sensors][daily] error", "err", err)
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	stdFormatted := fmt.Sprintf(
		"%d,%d,%d,%d,%d,%d",
		state.Config.TimeStd.Hemisphere,
		state.Config.TimeStd.Week,
		state.Config.TimeStd.Month,
		state.Config.TimeStd.Day,
		state.Config.TimeStd.Hour,
		state.Config.TimeStd.Offset,
	)
	dstFormatted := fmt.Sprintf(
		"%d,%d,%d,%d,%d,%d",
		state.Config.TimeDst.Hemisphere,
		state.Config.TimeDst.Week,
		state.Config.TimeDst.Month,
		state.Config.TimeDst.Day,
		state.Config.TimeDst.Hour,
		state.Config.TimeDst.Offset,
	)

	timezone := c.timezones.GetByParameters(*state.Config.Timezone, stdFormatted, dstFormatted)

	cfg := DeviceConfig{
		LedState:   state.Config.LedState,
		LedPower:   state.Config.LedPower,
		TelePeriod: state.Config.TelePeriod,
		LedPwmMode: state.Config.LedPwmMode,
		LedPwmOn:   state.Config.LedPwmOn,
		LedPwmOff:  state.Config.LedPwmOff,
		Timezone:   timezone,
	}

	slog.Info("[device][control-get] success")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(cfg)
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
	// Device takes up to 3 sec to get data after control.
	time.Sleep(3 * time.Second)
	// Updating the state
	c.pub.GetSensors(device.Device)

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
	case "led-pwm-mode":
		c.pub.SetLedPwmMode(d.Device, r.Value == "ON")
	case "led-pwm-off":
		value, err := strconv.Atoi(r.Value)
		if err != nil {
			return err
		}
		c.pub.SetLedPwmOn(d.Device, value)
	case "led-pwm-on":
		value, err := strconv.Atoi(r.Value)
		if err != nil {
			return err
		}
		c.pub.SetLedPwmOn(d.Device, value)
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
