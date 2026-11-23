package internal

import (
	"errors"
	"log/slog"
	"math"
	"smart-home/event"
	"smart-home/model"
	"time"
)

const (
	// mqWindow: Rs is averaged over the readings of the last minute (the sensor is noisy).
	mqWindow = time.Minute
	// mqFresh: calibrating needs a reading at least this recent.
	mqFresh = 3 * time.Minute
	// MQ-135 CO₂ curve exponent (ppm = a·(Rs/R0)^-b), only used to calibrate against a known
	// CO₂ value: the reading is never turned into ppm.
	mqCurveB = 2.769034857
	// mqAirPpm: outdoor CO₂, the air that Ratio 1.0 stands for.
	mqAirPpm = 420.0
)

type mqSample struct {
	at time.Time
	rs float32 // corrected
}

var (
	ErrMqDisabled  = errors.New("MQ sensor is not enabled")
	ErrMqNoReading = errors.New("no recent MQ reading")
)

// HasMq: the device has an MQ-xxx gas sensor set up and enabled.
func (s *DeviceState) HasMq() bool {
	return s.Device.Mq != nil && s.Device.Mq.Enabled
}

// ComputeMq turns the raw analog value of a telemetry message into an MQ reading (e.Mq, s.Mq)
// for devices with the sensor enabled. Call it before the message is used (detection, storage,
// WebSocket), so the reading travels with it.
//
// Rs = (Vcc − Vout) / Vout × RL, where Vout = raw / adcMax × adcVolts / divider and Vcc is the
// DC monitor's voltage (INA219 on the supply) or the configured supply. Rs is corrected for
// temperature and humidity when the message has them, averaged over the last minute, and
// Ratio = R0 / Rs.
func (s *DeviceState) ComputeMq(e *event.SensorEvent, now time.Time) {
	if e == nil || !s.HasMq() {
		return
	}
	cfg := s.Device.Mq
	raw, ok := e.Analog[cfg.Input]
	if !ok {
		return
	}

	volts := raw / cfg.AdcMax * cfg.AdcVolts / cfg.Divider
	vcc, vccDc := cfg.SupplyVolts, false
	if e.Dc != nil && e.Dc.Voltage > 1 {
		vcc, vccDc = e.Dc.Voltage, true
	}
	if volts <= 0 || volts >= vcc {
		slog.Debug("[device][mq] reading out of range", "deviceId", s.Device.ID, "raw", raw, "volts", volts, "vcc", vcc)
		return
	}
	rs := (vcc - volts) / volts * cfg.LoadOhm

	corrected := rs
	r := e.Readings()
	if r.Temperature != nil && r.Humidity != nil {
		if f := mqCorrection(*r.Temperature, *r.Humidity); f > 0.1 {
			corrected = rs / f
		}
	}

	s.mqSamples = append(s.mqSamples, mqSample{at: now, rs: corrected})
	for len(s.mqSamples) > 1 && now.Sub(s.mqSamples[0].at) > mqWindow {
		s.mqSamples = s.mqSamples[1:]
	}
	var sum float32
	for _, sample := range s.mqSamples {
		sum += sample.rs
	}
	avg := sum / float32(len(s.mqSamples))

	reading := &event.MqReading{Raw: raw, Volts: volts, Vcc: vcc, VccDc: vccDc, Rs: rs, RsAvg: avg}
	if cfg.R0 > 0 {
		reading.Ratio = new(round2(cfg.R0 / avg))
	}
	e.Mq = reading
	s.Mq = reading
	s.MqAt = now
}

// mqCorrection is the MQ-135 temperature/humidity dependency (GeorgK MQ135 library):
// Rs at t/h divided by Rs at 20 °C / 33 %.
func mqCorrection(t, h float32) float32 {
	if t < 20 {
		return 0.00035*t*t - 0.02718*t + 1.39538 - (h-33)*0.0018
	}
	return -0.003333333*t - 0.001923077*h + 1.130128205128
}

func round2(v float32) float32 {
	return float32(math.Round(float64(v)*100) / 100)
}

// ClearMq forgets the live MQ reading, e.g. when the device goes offline.
func (s *DeviceState) ClearMq() {
	s.Mq = nil
	s.mqSamples = nil
}

// CalibrateMq sets R0 and reports the new value:
//   - air: the current (averaged) Rs, so the current air becomes Ratio 1.0;
//   - ppm: from the current Rs and a known CO₂ value (e.g. a reference sensor), as if the
//     air were outdoor air; only valid when CO₂ is the only gas around;
//   - r0: the given value (e.g. restoring the previous one).
//
// The caller saves the device.
func (s *DeviceState) CalibrateMq(mode string, value float64, now time.Time) (float32, error) {
	if !s.HasMq() {
		return 0, ErrMqDisabled
	}
	var r0 float32
	switch mode {
	case "r0":
		r0 = float32(value)
	case "air", "ppm":
		if s.Mq == nil || now.Sub(s.MqAt) > mqFresh {
			return 0, ErrMqNoReading
		}
		r0 = s.Mq.RsAvg
		if mode == "ppm" {
			// Rs ∝ ppm^(−1/b): scale the current Rs to what it would be at mqAirPpm
			r0 *= float32(math.Pow(value/mqAirPpm, 1/mqCurveB))
		}
	default:
		return 0, errors.New("unknown mode " + mode)
	}
	r0 = float32(math.Round(float64(r0)))

	c := s.Device.Mq
	slog.Info("[device][mq] calibrated", "deviceId", s.Device.ID, "mode", mode, "value", value, "from", c.R0, "to", r0)
	if c.R0 > 0 && c.R0 != r0 {
		c.PrevR0 = new(c.R0)
	}
	c.R0 = r0
	c.ChangedAt = new(now.UTC())

	if s.Mq != nil && s.Mq.RsAvg > 0 {
		s.Mq.Ratio = new(round2(r0 / s.Mq.RsAvg))
	}
	return r0, nil
}

// SetMqSettings replaces the hardware settings, keeping the calibration. A changed load
// resistor or ADC setting rescales Rs, so the old R0 no longer fits: the UI says to recalibrate.
func (s *DeviceState) SetMqSettings(settings model.MqConfig) {
	cfg := model.DefaultMqConfig()
	if s.Device.Mq != nil {
		cfg = *s.Device.Mq
	}
	cfg.Enabled = settings.Enabled
	cfg.Input = settings.Input
	cfg.LoadOhm = settings.LoadOhm
	cfg.AdcVolts = settings.AdcVolts
	cfg.AdcMax = settings.AdcMax
	cfg.Divider = settings.Divider
	cfg.SupplyVolts = settings.SupplyVolts
	s.Device.Mq = &cfg
	s.ClearMq()
}
