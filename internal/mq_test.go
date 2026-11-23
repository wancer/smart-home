package internal

import (
	"encoding/json"
	"errors"
	"math"
	"smart-home/event"
	"smart-home/model"
	"testing"
	"time"
)

// The board from the issue: FC-22 module (1 kΩ load) on GPIO34 of an ESP32, INA219 on the 5 V rail, BME280.
const mqPayload = `{"Time":"2026-11-22T22:08:58","ANALOG":{"A1":203},
	"BME280":{"Temperature":4.7,"Humidity":74.3,"DewPoint":0.5,"Pressure":976.3},
	"INA219":{"Id":40,"Voltage":4.96,"Current":0.194,"Power":0.96},"ESP32":{"Temperature":62.2}}`

func mqEvent(t *testing.T) *event.SensorEvent {
	t.Helper()
	var e event.SensorEvent
	if err := json.Unmarshal([]byte(mqPayload), &e); err != nil {
		t.Fatal(err)
	}
	return &e
}

func mqState() *DeviceState {
	cfg := model.DefaultMqConfig()
	cfg.Enabled = true
	return &DeviceState{Device: &model.Device{ID: 1, Mq: &cfg}, Config: &DeviceConfig{}}
}

func near(t *testing.T, name string, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Errorf("%s = %v, want %v ± %v", name, got, want, tolerance)
	}
}

func TestComputeMq(t *testing.T) {
	s := mqState()
	now := time.Now()
	e := mqEvent(t)
	s.ComputeMq(e, now)

	m := e.Mq
	if m == nil || s.Mq != m {
		t.Fatalf("no MQ reading: %+v", m)
	}
	// Vout = 203 / 4095 × 3.3 = 0.1636 V; Rs = (4.96 − 0.1636) / 0.1636 × 1000
	near(t, "Volts", float64(m.Volts), 0.1636, 0.0005)
	if !m.VccDc || m.Vcc != 4.96 {
		t.Errorf("Vcc = %v (dc %v), want the INA219 4.96 V", m.Vcc, m.VccDc)
	}
	near(t, "Rs", float64(m.Rs), 29319, 30)
	// 4.7 °C / 74.3 %: correction 1.2010
	near(t, "RsAvg", float64(m.RsAvg), 29319/1.2010, 30)
	if m.Ratio != nil {
		t.Error("not calibrated: no ratio")
	}
	if r := e.Readings(); r.GasRs == nil || r.GasRatio != nil {
		t.Errorf("readings: GasRs %v, GasRatio %v", r.GasRs, r.GasRatio)
	}
}

func TestComputeMqDisabledOrMissing(t *testing.T) {
	s := mqState()
	s.Device.Mq.Enabled = false
	e := mqEvent(t)
	s.ComputeMq(e, time.Now())
	if e.Mq != nil {
		t.Error("disabled sensor must not compute")
	}

	s = mqState()
	s.Device.Mq.Input = "A2"
	s.ComputeMq(e, time.Now())
	if e.Mq != nil {
		t.Error("missing input must not compute")
	}
}

func TestCalibrateMq(t *testing.T) {
	s := mqState()
	now := time.Now()

	if _, err := s.CalibrateMq("air", 0, now); !errors.Is(err, ErrMqNoReading) {
		t.Fatalf("air without a reading: err = %v", err)
	}

	e := mqEvent(t)
	s.ComputeMq(e, now)
	r0, err := s.CalibrateMq("air", 0, now)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "R0 (air)", float64(r0), float64(s.Mq.RsAvg), 1)
	near(t, "Ratio after air calibration", float64(*s.Mq.Ratio), 1, 0.001)
	if s.Device.Mq.PrevR0 != nil {
		t.Error("first calibration has no previous R0")
	}

	// against 840 ppm: Rs at 420 ppm would be Rs × 2^(1/b)
	r0ppm, err := s.CalibrateMq("ppm", 840, now)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "R0 (ppm)", float64(r0ppm), float64(s.Mq.RsAvg)*math.Pow(2, 1/mqCurveB), 1)
	if p := s.Device.Mq.PrevR0; p == nil || *p != r0 {
		t.Errorf("PrevR0 = %v, want %v", p, r0)
	}

	// restore
	if _, err := s.CalibrateMq("r0", float64(r0), now); err != nil {
		t.Fatal(err)
	}
	if s.Device.Mq.R0 != r0 || *s.Device.Mq.PrevR0 != r0ppm {
		t.Errorf("restore: R0 %v prev %v", s.Device.Mq.R0, *s.Device.Mq.PrevR0)
	}

	// the next telemetry uses the new R0
	e2 := mqEvent(t)
	s.ComputeMq(e2, now.Add(10*time.Second))
	near(t, "Ratio", float64(*e2.Mq.Ratio), 1, 0.01)

	if _, err := s.CalibrateMq("air", 0, now.Add(time.Hour)); !errors.Is(err, ErrMqNoReading) {
		t.Errorf("stale reading: err = %v", err)
	}
}

func TestMqAveragesTheLastMinute(t *testing.T) {
	s := mqState()
	now := time.Now()
	for i := range 10 {
		s.ComputeMq(mqEvent(t), now.Add(time.Duration(i)*20*time.Second))
	}
	if n := len(s.mqSamples); n != 4 { // 120 s … 180 s: the last minute and its first sample
		t.Errorf("samples kept = %d, want 4", n)
	}
}
