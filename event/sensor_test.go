package event

import (
	"encoding/json"
	"testing"
)

func TestReadingsBME280AndINA219(t *testing.T) {
	payload := `{"Time":"2026-11-20T12:00:00",
		"BME280":{"Temperature":25.6,"Humidity":32.2,"DewPoint":7.7,"Pressure":975.6},
		"INA219":{"Id":40,"Voltage":5.012,"Current":0.206,"Power":1.033},
		"ESP32":{"Temperature":41.1},"PressureUnit":"hPa","TempUnit":"C"}`

	var e SensorEvent
	if err := json.Unmarshal([]byte(payload), &e); err != nil {
		t.Fatal(err)
	}
	r := e.Readings()

	check := func(name string, got *float32, want float32) {
		t.Helper()
		if got == nil || *got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	check("Temperature", r.Temperature, 25.6)
	check("Humidity", r.Humidity, 32.2)
	check("DewPoint", r.DewPoint, 7.7)
	check("Pressure", r.Pressure, 975.6)
	check("DcVoltage", r.DcVoltage, 5.012)
	check("DcCurrent", r.DcCurrent, 0.206)
	check("DcPower", r.DcPower, 1.033)
	if r.Power != nil || r.Voltage != nil || r.CO2 != nil {
		t.Errorf("unexpected AC/CO2 readings: %+v", r)
	}
}

func TestReadingsClimatePrecedence(t *testing.T) {
	payload := `{"Time":"2026-11-20T12:00:00",
		"SCD40":{"CarbonDioxide":600,"Temperature":30,"Humidity":40,"DewPoint":15},
		"BME280":{"Temperature":21,"Humidity":45,"DewPoint":9,"Pressure":1000}}`

	var e SensorEvent
	if err := json.Unmarshal([]byte(payload), &e); err != nil {
		t.Fatal(err)
	}
	r := e.Readings()
	if *r.Temperature != 21 || *r.Humidity != 45 {
		t.Errorf("BME280 should win over SCD40, got %v / %v", *r.Temperature, *r.Humidity)
	}
	if *r.CO2 != 600 || *r.Pressure != 1000 {
		t.Errorf("CO2 / pressure lost: %+v", r)
	}
}

func TestReadingsNil(t *testing.T) {
	var e *SensorEvent
	if r := e.Readings(); r.Temperature != nil {
		t.Error("nil event must have no readings")
	}
}
