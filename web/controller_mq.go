package web

import (
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"smart-home/internal"
	"smart-home/model"
	"time"
)

// MqSettings are the hardware settings of an MQ-xxx gas sensor on an analog input.
type MqSettings struct {
	Enabled     bool    `json:"enabled"`
	Input       string  `json:"input"`       // ANALOG key, e.g. A1
	LoadOhm     float32 `json:"loadOhm"`     // load resistor on the module, Ω
	AdcVolts    float32 `json:"adcVolts"`    // ADC full scale, V
	AdcMax      float32 `json:"adcMax"`      // raw value at full scale
	Divider     float32 `json:"divider"`     // GPIO voltage / module output; 1 = none
	SupplyVolts float32 `json:"supplyVolts"` // module supply when no DC monitor reports it, V
}

// MqConfig is the calibration popup: settings, the live reading and the calibration.
type MqConfig struct {
	Settings MqSettings `json:"settings"`
	Inputs   []string   `json:"inputs"` // ANALOG keys in the last telemetry (A1, ...)

	Raw       *float32 `json:"raw"`       // ADC value; null without a reading
	Volts     *float32 `json:"volts"`     // module output, V
	Vcc       *float32 `json:"vcc"`       // module supply used, V
	VccSource *string  `json:"vccSource"` // "dc" (DC monitor, e.g. INA219) or "fixed"
	Rs        *float32 `json:"rs"`        // Ω, uncorrected
	RsAvg     *float32 `json:"rsAvg"`     // Ω, temperature/humidity corrected, 1 min average
	Ratio     *float32 `json:"ratio"`     // R0 / RsAvg; null without a reading or not calibrated

	R0        float32  `json:"r0"`        // Ω; 0 = not calibrated
	PrevR0    *float32 `json:"prevR0"`    // Ω before the last change
	ChangedAt *int64   `json:"changedAt"` // unix time of the last R0 change
}

type MqCalibrateRequest struct {
	Mode  string  `json:"mode"`  // air (current air = 1.0), ppm (against a known CO₂ value), r0 (set R0)
	Value float64 `json:"value"` // ppm or Ω; unused for air
}

func newMqConfig(state *internal.DeviceState) MqConfig {
	c := model.DefaultMqConfig()
	if state.Device.Mq != nil {
		c = *state.Device.Mq
	}
	cfg := MqConfig{
		Settings: MqSettings{
			Enabled: c.Enabled, Input: c.Input, LoadOhm: c.LoadOhm, AdcVolts: c.AdcVolts,
			AdcMax: c.AdcMax, Divider: c.Divider, SupplyVolts: c.SupplyVolts,
		},
		Inputs: slices.Sorted(maps.Keys(state.Analog)),
		R0:     c.R0,
		PrevR0: c.PrevR0,
	}
	if c.ChangedAt != nil {
		cfg.ChangedAt = new(c.ChangedAt.Unix())
	}
	if m := state.Mq; m != nil {
		cfg.Raw, cfg.Volts, cfg.Vcc = new(m.Raw), new(m.Volts), new(m.Vcc)
		cfg.Rs, cfg.RsAvg, cfg.Ratio = new(m.Rs), new(m.RsAvg), m.Ratio
		source := "fixed"
		if m.VccDc {
			source = "dc"
		}
		cfg.VccSource = &source
	}
	return cfg
}

func (c *DeviceControlController) GetMq(w http.ResponseWriter, r *http.Request) {
	state := deviceState(w, r, c.states)
	if state == nil {
		return
	}
	if state.Online {
		c.pub.GetSensors(state.Device) // fresh ANALOG reading
		time.Sleep(2 * time.Second)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newMqConfig(state))
}

// SetMq saves the hardware settings (enabling/disabling the sensor), keeping the calibration.
func (c *DeviceControlController) SetMq(w http.ResponseWriter, r *http.Request) {
	state := deviceState(w, r, c.states)
	if state == nil {
		return
	}
	var req MqSettings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	settings := model.MqConfig{
		Enabled: req.Enabled, Input: req.Input, LoadOhm: req.LoadOhm, AdcVolts: req.AdcVolts,
		AdcMax: req.AdcMax, Divider: req.Divider, SupplyVolts: req.SupplyVolts,
	}
	if err := model.ValidateMqSettings(&settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	state.SetMqSettings(settings)
	if err := c.profiles.SaveMq(state.Device); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	slog.Info("[device][mq] settings saved", "deviceId", state.Device.ID, "enabled", req.Enabled)

	if state.Online {
		c.pub.GetSensors(state.Device) // first reading with the new settings
		time.Sleep(2 * time.Second)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newMqConfig(state))
}

// CalibrateMq sets R0 in the backend (nothing is sent to the device): 200 with the new config,
// 400 bad request or sensor not enabled, 409 no recent reading (offline or no ANALOG value).
func (c *DeviceControlController) CalibrateMq(w http.ResponseWriter, r *http.Request) {
	state := deviceState(w, r, c.states)
	if state == nil {
		return
	}

	var req MqCalibrateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	switch req.Mode {
	case "air":
	case "ppm":
		if req.Value < 300 || req.Value > 10000 {
			http.Error(w, "ppm must be 300..10000", http.StatusBadRequest)
			return
		}
	case "r0":
		if req.Value < 1 || req.Value > 10_000_000 {
			http.Error(w, "r0 must be 1..10000000", http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, "mode must be air, ppm or r0", http.StatusBadRequest)
		return
	}

	r0, err := state.CalibrateMq(req.Mode, req.Value, time.Now())
	switch {
	case errors.Is(err, internal.ErrMqDisabled):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case errors.Is(err, internal.ErrMqNoReading):
		http.Error(w, err.Error(), http.StatusConflict)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := c.profiles.SaveMq(state.Device); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	slog.Info("[device][mq-calibrate] success", "deviceId", state.Device.ID, "mode", req.Mode, "value", req.Value, "r0", r0)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newMqConfig(state))
}
