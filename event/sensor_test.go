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

func TestAnalogIsKeptRaw(t *testing.T) {
	payload := `{"Time":"2026-11-22T22:08:58","ANALOG":{"A1":203,"Range1":"x"},
		"BME280":{"Temperature":4.7,"Humidity":74.3,"DewPoint":0.5,"Pressure":976.3}}`

	var e SensorEvent
	if err := json.Unmarshal([]byte(payload), &e); err != nil {
		t.Fatal(err)
	}
	if e.Analog["A1"] != 203 || len(e.Analog) != 1 {
		t.Errorf("Analog = %v, want only A1 = 203", e.Analog)
	}
	if e.Mq != nil || e.Readings().GasRatio != nil {
		t.Error("an MQ reading is only computed by the backend")
	}
	if e.TempHum == nil || e.TempHum.Temperature != 4.7 {
		t.Errorf("BME280 lost: %+v", e.TempHum)
	}
}

func TestReadingsBH1750AndTCS34725(t *testing.T) {
	payload := `{"Time":"2026-11-23T17:28:42","ts":1795447722,"ESP32":{"Temperature":18.33333},
		"BH1750":{"Illuminance":223.5482},
		"TCS34725":{"Red":41.78241,"Green":34.01711,"Blue":22.79176,"Clear":30.87053,"Illuminance":188.7146,"ColorTemperature":3469.305}}`

	var e SensorEvent
	if err := json.Unmarshal([]byte(payload), &e); err != nil {
		t.Fatal(err)
	}
	if e.Light == nil || e.Color == nil {
		t.Fatalf("light / colour sensor not recognised: %+v", e)
	}
	if e.Color.Red != 41.78241 || e.Color.Clear != 30.87053 {
		t.Errorf("colour = %+v", e.Color)
	}
	r := e.Readings()
	if r.Illuminance == nil || *r.Illuminance != 223.5482 {
		t.Errorf("Illuminance = %v, want the BH1750 value", r.Illuminance)
	}
	if r.ColorTemp == nil || *r.ColorTemp != 3469.305 {
		t.Errorf("ColorTemp = %v", r.ColorTemp)
	}
	if r.Temperature != nil || r.Power != nil {
		t.Errorf("unexpected readings: %+v", r)
	}

	// the colour sensor alone also gives the illuminance
	var c SensorEvent
	if err := json.Unmarshal([]byte(`{"TCS34725":{"Red":1,"Green":2,"Blue":3,"Clear":4,"Illuminance":188.5,"ColorTemperature":3000}}`), &c); err != nil {
		t.Fatal(err)
	}
	if r := c.Readings(); r.Illuminance == nil || *r.Illuminance != 188.5 {
		t.Errorf("Illuminance = %v, want the TCS34725 value", r.Illuminance)
	}
}

func TestRawValues(t *testing.T) {
	payload := `{"Time":"2026-11-23T17:28:42","TempUnit":"C",
		"TCS34725":{"Red":41.78,"Green":34.01,"Blue":22.79,"Clear":30.87,"Illuminance":188.71,"ColorTemperature":3469.3},
		"PZEM":{"Name":"phase","Voltage":[230,231],"Power":12}}`

	var e SensorEvent
	if err := json.Unmarshal([]byte(payload), &e); err != nil {
		t.Fatal(err)
	}
	if got := e.Raw["TCS34725"]["Clear"]; got != 30.87 {
		t.Errorf("TCS34725.Clear = %v", got)
	}
	if e.Raw["PZEM"]["Voltage1"] != 230 || e.Raw["PZEM"]["Voltage2"] != 231 || e.Raw["PZEM"]["Power"] != 12 {
		t.Errorf("PZEM = %v", e.Raw["PZEM"])
	}
	if _, ok := e.Raw["PZEM"]["Name"]; ok {
		t.Error("strings must be skipped")
	}
}
