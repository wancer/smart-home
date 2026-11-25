package web

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"smart-home/config"
	"smart-home/internal"
	"smart-home/model"
	"smart-home/mqtt"
	"strconv"
	"time"
)

type DeviceControlController struct {
	pub       *mqtt.Publisher
	states    *internal.DeviceStateManager
	timezones *config.TimezonesConfig
	firmware  *internal.FirmwareChecker
	profiles  *internal.DeviceProfiles
}

func NewDeviceControlController(
	pub *mqtt.Publisher,
	states *internal.DeviceStateManager,
	timezones *config.TimezonesConfig,
	firmware *internal.FirmwareChecker,
	profiles *internal.DeviceProfiles,
) *DeviceControlController {
	return &DeviceControlController{
		pub:       pub,
		states:    states,
		timezones: timezones,
		firmware:  firmware,
		profiles:  profiles,
	}
}

type DeviceControlRequest struct {
	DeviceId  uint   `json:"deviceId"`
	Parameter string `json:"parameter"`
	Value     string `json:"value"`
	// Relay is the POWER<n> output "on-off" switches on a multi-relay device (0 or 1 = POWER).
	Relay int `json:"relay,omitempty"`
}

func (c *DeviceControlController) resolveTimezone(state *internal.DeviceState) *string {
	var stdFormatted, dstFormatted, offset string
	if state.Config.TimeStd != nil {
		stdFormatted = fmt.Sprintf(
			"%d,%d,%d,%d,%d,%d",
			state.Config.TimeStd.Hemisphere,
			state.Config.TimeStd.Week,
			state.Config.TimeStd.Month,
			state.Config.TimeStd.Day,
			state.Config.TimeStd.Hour,
			state.Config.TimeStd.Offset,
		)
	}
	if state.Config.TimeDst != nil {
		dstFormatted = fmt.Sprintf(
			"%d,%d,%d,%d,%d,%d",
			state.Config.TimeDst.Hemisphere,
			state.Config.TimeDst.Week,
			state.Config.TimeDst.Month,
			state.Config.TimeDst.Day,
			state.Config.TimeDst.Hour,
			state.Config.TimeDst.Offset,
		)
	}
	if state.Config.Timezone != nil {
		offset = *state.Config.Timezone
	}
	return c.timezones.GetByParameters(offset, stdFormatted, dstFormatted)
}

func (c *DeviceControlController) Get(w http.ResponseWriter, r *http.Request) {
	state := deviceState(w, r, c.states)
	if state == nil {
		return
	}

	cfg := DeviceConfig{
		TelePeriod: state.Config.TelePeriod,
		LedConfig:  newLedConfig(state),
		Timezone:   c.resolveTimezone(state),
		Firmware:   NewFirmwareConfig(state.Device, c.firmware.Latest()),
		Hardware:   chip(state.Device),
	}

	slog.Info("[device][control-get] success")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(cfg)
}

func (c *DeviceControlController) GetLed(w http.ResponseWriter, r *http.Request) {
	state := tasmotaDevice(w, r, c.states)
	if state == nil {
		return
	}

	// The stored template says which LEDs / lights the device has; ask for it if unknown.
	if state.Device.Template == nil {
		c.pub.GetTemplate(state.Device)
		time.Sleep(1500 * time.Millisecond)
	}

	hw := state.LedHardware()
	if hw == nil || hw.StatusLed {
		c.pub.GetLedPower(state.Device)
		c.pub.GetLedState(state.Device)
		c.pub.GetLedPwmMode(state.Device)
		c.pub.GetLedPwmOn(state.Device)
		c.pub.GetLedPwmOff(state.Device)
	}
	if hw != nil && hw.Light != internal.LightNone {
		c.pub.GetLight(state.Device, hw.Light == internal.LightRGB)
	}
	time.Sleep(1500 * time.Millisecond)

	cfg := newLedConfig(state)
	slog.Info("[device][led-get] success", "deviceId", state.Device.ID)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(cfg)
}

func (c *DeviceControlController) GetTiming(w http.ResponseWriter, r *http.Request) {
	state := deviceState(w, r, c.states)
	if state == nil {
		return
	}

	if state.Device.IsTasmota() { // others: what they published (tele/STATE)
		c.pub.GetTelePeriod(state.Device)
		c.pub.GetTimezone(state.Device)
		c.pub.GetTimeStd(state.Device)
		c.pub.GetTimeDst(state.Device)
		c.pub.GetStatus(state.Device) // uptime
		time.Sleep(3 * time.Second)
	}

	cfg := TimingConfig{
		TelePeriod:     state.Config.TelePeriod,
		Timezone:       c.resolveTimezone(state),
		UptimeSec:      uptime(state),
		OfflineTimeout: uint(internal.OfflineTimeout(state) / time.Second),
	}

	slog.Info("[device][timing-get] success", "deviceId", state.Device.ID)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(cfg)
}

func (c *DeviceControlController) GetHardware(w http.ResponseWriter, r *http.Request) {
	state := deviceState(w, r, c.states)
	if state == nil {
		return
	}

	// chip and firmware are in the stored profile (POST /profile/refresh re-reads them)
	if state.Device.IsTasmota() { // others: what they published
		c.pub.GetSensors(state.Device) // refreshes the chip temperature
		c.pub.GetStatus(state.Device)  // load average
		time.Sleep(3 * time.Second)
	}

	cfg := HardwareConfig{
		Hardware:        chip(state.Device),
		ChipTemperature: state.ChipTemperature,
		LoadAvg:         state.LoadAvg,
		Firmware:        NewFirmwareConfig(state.Device, c.firmware.Latest()),
	}

	slog.Info("[device][hardware-get] success", "deviceId", state.Device.ID)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(cfg)
}

func chip(d *model.Device) *string {
	if d.Chip == "" {
		return nil
	}
	return new(d.Chip)
}

func uptime(state *internal.DeviceState) *uint64 {
	if state.Uptime == nil {
		return nil
	}
	return new(state.Uptime.Current())
}

// UpgradeFirmware only triggers the upgrade: the device downloads the image and restarts on its own.
func (c *DeviceControlController) UpgradeFirmware(w http.ResponseWriter, r *http.Request) {
	state := tasmotaDevice(w, r, c.states)
	if state == nil {
		return
	}
	if !state.Online {
		slog.Warn("[device][firmware-upgrade] device is offline", "deviceId", state.Device.ID)
		http.Error(w, http.StatusText(http.StatusConflict), http.StatusConflict)
		return
	}

	c.pub.Upgrade(state.Device)

	slog.Info("[device][firmware-upgrade] success", "deviceId", state.Device.ID)
	w.WriteHeader(http.StatusOK)
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

	if !device.Device.IsTasmota() {
		slog.Warn("[device][control] not a Tasmota device", "deviceId", parsed.DeviceId, "platform", device.Device.Platform)
		http.Error(w, http.StatusText(http.StatusConflict), http.StatusConflict)
		return
	}

	if !supportsParameter(device, parsed.Parameter) {
		slog.Error("[device][control] unsupported for device", "deviceId", parsed.DeviceId, "sensorType", device.Device.SensorType, "parameter", parsed.Parameter)
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
		if r.Relay < 0 || r.Relay > max(1, d.RelayCount()) {
			return fmt.Errorf("invalid relay: %d", r.Relay)
		}
		c.pub.RelayOnOff(d.Device, r.Relay, r.Value == "ON")
	case "voltage":
		volts, err := strconv.Atoi(r.Value)
		if err != nil {
			return err
		}
		c.pub.SetVoltage(d.Device, volts)
	case "tele-period":
		period, err := strconv.Atoi(r.Value)
		if err != nil {
			return err
		}
		c.pub.SetTelePeriod(d.Device, period)
	case "timezone":
		timezone, err := c.timezones.Map(r.Value)
		if err != nil {
			return err
		}
		c.pub.SetTimezone(d.Device, timezone.Offset)
		c.pub.SetTimeDst(d.Device, timezone.TimeDst)
		c.pub.SetTimeStd(d.Device, timezone.TimeStd)
	case "led-power":
		c.pub.SetLedPower(d.Device, r.Value == "ON")
	case "led-mode":
		value, err := strconv.Atoi(r.Value)
		if err != nil || value < 0 || value > 8 {
			return fmt.Errorf("invalid led mode: %s", r.Value)
		}
		c.pub.SetLedState(d.Device, value)
	case "led-pwm-mode":
		c.pub.SetLedPwmMode(d.Device, r.Value == "ON")
	case "led-pwm-off":
		value, err := strconv.Atoi(r.Value)
		if err != nil {
			return err
		}
		c.pub.SetLedPwmOff(d.Device, value)
	case "led-pwm-on":
		value, err := strconv.Atoi(r.Value)
		if err != nil {
			return err
		}
		c.pub.SetLedPwmOn(d.Device, value)
	case "light-power":
		c.pub.SetLightPower(d.Device, d.LightPowerIndex(), r.Value == "ON")
	case "light-dimmer":
		value, err := strconv.Atoi(r.Value)
		if err != nil || value < 0 || value > 100 {
			return fmt.Errorf("invalid dimmer: %s", r.Value)
		}
		c.pub.SetDimmer(d.Device, value)
	case "light-color":
		hue, sat, err := internal.HexToHueSat(r.Value)
		if err != nil {
			return err
		}
		c.pub.SetHueSat(d.Device, hue, sat)
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

// supportsParameter: calibration is for devices with an energy meter only, power control for devices with a relay.
func supportsParameter(state *internal.DeviceState, parameter string) bool {
	switch parameter {
	case "voltage", "power":
		return state.HasEnergyMeter()
	case "on-off":
		return state.CanToggle()
	}
	return true
}

func newLedConfig(state *internal.DeviceState) LedConfig {
	cfg := LedConfig{
		LedState:   state.Config.LedState,
		LedPower:   state.Config.LedPower,
		LedPwmOn:   state.Config.LedPwmOn,
		LedPwmOff:  state.Config.LedPwmOff,
		LedPwmMode: state.Config.LedPwmMode,
		StatusLed:  true, // template unknown: offer the status LED settings as before
		LightPower: state.Config.LightPower,
		Dimmer:     state.Config.Dimmer,
	}
	if hw := state.LedHardware(); hw != nil {
		cfg.StatusLed = hw.StatusLed
		if hw.Light != internal.LightNone {
			cfg.Light = new(hw.Light)
		}
	}
	if state.Config.Hue != nil && state.Config.Saturation != nil {
		cfg.Color = new(internal.HueSatToHex(*state.Config.Hue, *state.Config.Saturation))
	}
	return cfg
}
