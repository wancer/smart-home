package event

import (
	"encoding/json"
	"math"
	"testing"

	"smart-home/model"
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

// series is a SeriesReadings entry with its kind, for comparing in tests.
type series struct {
	Source, Field string
	Value         float64
	Kind          string
}

func seriesOf(t *testing.T, payload string) []series {
	t.Helper()
	var e SensorEvent
	if err := json.Unmarshal([]byte(payload), &e); err != nil {
		t.Fatal(err)
	}
	return seriesFrom(&e)
}

func seriesFrom(e *SensorEvent) []series {
	var out []series
	for _, r := range e.SeriesReadings() {
		out = append(out, series{r.Source, r.Field, r.Value, model.SeriesKind(r.Source, r.Field)})
	}
	return out
}

func checkSeries(t *testing.T, got, want []series) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d series, want %d:\n got  %+v\n want %+v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("series %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// The sample payload of docs/tasks/sensor-series.md: both illuminances are kept.
func TestSeriesReadingsTaskSample(t *testing.T) {
	got := seriesOf(t, `{"Time":"2026-11-23T17:28:42","ts":1795447722,"ESP32":{"Temperature":18.33333},
		"BH1750":{"Illuminance":223.5482},
		"TCS34725":{"Red":41.78241,"Green":34.01711,"Blue":22.79176,"Clear":30.87053,"Illuminance":188.7146,"ColorTemperature":3469.305}}`)
	checkSeries(t, got, []series{
		{"BH1750", "Illuminance", 223.5482, model.KindIlluminance},
		{"ESP32", "Temperature", 18.33333, model.KindChipTemperature},
		{"TCS34725", "Blue", 22.79176, model.KindColorBlue},
		{"TCS34725", "Clear", 30.87053, model.KindColorClear},
		{"TCS34725", "ColorTemperature", 3469.305, model.KindColorTemp},
		{"TCS34725", "Green", 34.01711, model.KindColorGreen},
		{"TCS34725", "Illuminance", 188.7146, model.KindIlluminance},
		{"TCS34725", "Red", 41.78241, model.KindColorRed},
	})
}

func TestSeriesReadingsEnergyPlug(t *testing.T) {
	got := seriesOf(t, `{"Time":"2026-11-20T12:00:00","ENERGY":{"TotalStartTime":"2024-01-01T00:00:00",
		"Total":123.456,"Yesterday":1.2,"Today":0.5,"Period":3,"Power":45,"ApparentPower":50,
		"ReactivePower":20,"Factor":0.9,"Voltage":230,"Current":0.217}}`)
	checkSeries(t, got, []series{
		{"ENERGY", "ApparentPower", 50, model.KindApparentPower},
		{"ENERGY", "Current", 0.217, model.KindCurrent},
		{"ENERGY", "Factor", 0.9, model.KindPowerFactor},
		{"ENERGY", "Power", 45, model.KindPower},
		{"ENERGY", "ReactivePower", 20, model.KindReactivePower},
		{"ENERGY", "Voltage", 230, model.KindVoltage},
	})
}

func TestSeriesReadingsBME280AndINA219(t *testing.T) {
	got := seriesOf(t, `{"Time":"2026-11-20T12:00:00",
		"BME280":{"Temperature":25.6,"Humidity":32.2,"DewPoint":7.7,"Pressure":975.6},
		"INA219":{"Id":40,"Voltage":5.012,"Current":0.206,"Power":1.033},
		"ESP32":{"Temperature":41.1},"PressureUnit":"hPa","TempUnit":"C"}`)
	checkSeries(t, got, []series{
		{"BME280", "DewPoint", 7.7, model.KindDewPoint},
		{"BME280", "Humidity", 32.2, model.KindHumidity},
		{"BME280", "Pressure", 975.6, model.KindPressure},
		{"BME280", "Temperature", 25.6, model.KindTemperature},
		{"ESP32", "Temperature", 41.1, model.KindChipTemperature},
		{"INA219", "Current", 0.206, model.KindDcCurrent},
		{"INA219", "Power", 1.033, model.KindDcPower},
		{"INA219", "Voltage", 5.012, model.KindDcVoltage},
	})
}

// Both climate sensors are kept, no precedence.
func TestSeriesReadingsSCD40AndBME280(t *testing.T) {
	got := seriesOf(t, `{"Time":"2026-11-20T12:00:00",
		"SCD40":{"CarbonDioxide":600,"Temperature":30,"Humidity":40,"DewPoint":15},
		"BME280":{"Temperature":21,"Humidity":45,"DewPoint":9,"Pressure":1000}}`)
	checkSeries(t, got, []series{
		{"BME280", "DewPoint", 9, model.KindDewPoint},
		{"BME280", "Humidity", 45, model.KindHumidity},
		{"BME280", "Pressure", 1000, model.KindPressure},
		{"BME280", "Temperature", 21, model.KindTemperature},
		{"SCD40", "CarbonDioxide", 600, model.KindCo2},
		{"SCD40", "DewPoint", 15, model.KindDewPoint},
		{"SCD40", "Humidity", 40, model.KindHumidity},
		{"SCD40", "Temperature", 30, model.KindTemperature},
	})
}

func TestSeriesReadingsAnalogAndMq(t *testing.T) {
	var e SensorEvent
	payload := `{"Time":"2026-11-22T22:08:58","ANALOG":{"A1":203,"Range1":"x"},
		"BME280":{"Temperature":4.7,"Humidity":74.3,"DewPoint":0.5,"Pressure":976.3}}`
	if err := json.Unmarshal([]byte(payload), &e); err != nil {
		t.Fatal(err)
	}
	ratio := float32(1.5)
	e.Mq = &MqReading{Raw: 203, Rs: 1000, RsAvg: 1100, Ratio: &ratio}
	checkSeries(t, seriesFrom(&e), []series{
		{"ANALOG", "A1", 203, model.KindRaw},
		{"BME280", "DewPoint", 0.5, model.KindDewPoint},
		{"BME280", "Humidity", 74.3, model.KindHumidity},
		{"BME280", "Pressure", 976.3, model.KindPressure},
		{"BME280", "Temperature", 4.7, model.KindTemperature},
		{"MQ", "Ratio", 1.5, model.KindGasRatio},
		{"MQ", "Rs", 1000, model.KindRaw},
	})

	// not calibrated: Rs only
	e.Mq.Ratio = nil
	for _, s := range seriesFrom(&e) {
		if s.Source == "MQ" && s.Field == "Ratio" {
			t.Error("MQ.Ratio without calibration")
		}
	}

	// ANALOG values set without Raw (built by hand) still come through
	a := SensorEvent{Analog: map[string]float32{"A1": 512}}
	checkSeries(t, seriesFrom(&a), []series{{"ANALOG", "A1", 512, model.KindRaw}})
}

func TestSeriesReadingsMultiPhase(t *testing.T) {
	var e SensorEvent
	e.Raw = map[string]map[string]float64{
		"ENERGY": {"Voltage1": 230, "Voltage2": 231, "Power1": 10, "Power2": 20, "Today1": 1, "Total": 5},
		"PZEM":   {"Voltage1": 229},
	}
	checkSeries(t, seriesFrom(&e), []series{
		{"ENERGY", "Power1", 10, model.KindPower},
		{"ENERGY", "Power2", 20, model.KindPower},
		{"ENERGY", "Voltage1", 230, model.KindVoltage},
		{"ENERGY", "Voltage2", 231, model.KindVoltage},
		{"PZEM", "Voltage1", 229, model.KindRaw},
	})
}

func TestSeriesReadingsSkipsNaNAndNil(t *testing.T) {
	var n *SensorEvent
	if r := n.SeriesReadings(); r != nil {
		t.Errorf("nil event: %v", r)
	}
	e := SensorEvent{Raw: map[string]map[string]float64{
		"X": {"A": math.NaN(), "B": math.Inf(1), "C": 1},
	}}
	checkSeries(t, seriesFrom(&e), []series{{"X", "C", 1, model.KindRaw}})
}

func TestDS18B20(t *testing.T) {
	payload := `{"Time":"2026-11-24T12:00:00","DS18B20":{"Id":"0B257989FB50","Temperature":31.8},"TempUnit":"C"}`
	var e SensorEvent
	if err := json.Unmarshal([]byte(payload), &e); err != nil {
		t.Fatal(err)
	}
	if e.Thermo == nil || e.Thermo.Temperature != 31.8 || e.TempHum != nil {
		t.Fatalf("Thermo = %+v, TempHum = %+v", e.Thermo, e.TempHum)
	}
	r := e.Readings()
	if r.Temperature == nil || *r.Temperature != 31.8 || r.Humidity != nil {
		t.Errorf("readings: %+v", r)
	}
	checkSeries(t, seriesFrom(&e), []series{
		{"DS18B20", "Temperature", 31.8, model.KindTemperature},
	})
}

// Several DS18B20 on one bus: each its own series; a T-H sensor keeps precedence for Readings.
func TestDS18B20Multiple(t *testing.T) {
	got := seriesOf(t, `{"Time":"2026-11-24T12:00:00",
		"DS18B20-1":{"Id":"0B257989FB50","Temperature":31.8},
		"DS18B20-2":{"Id":"0C257989FB51","Temperature":5.2},
		"SHT3X":{"Temperature":21,"Humidity":45,"DewPoint":9}}`)
	checkSeries(t, got, []series{
		{"DS18B20-1", "Temperature", 31.8, model.KindTemperature},
		{"DS18B20-2", "Temperature", 5.2, model.KindTemperature},
		{"SHT3X", "DewPoint", 9, model.KindDewPoint},
		{"SHT3X", "Humidity", 45, model.KindHumidity},
		{"SHT3X", "Temperature", 21, model.KindTemperature},
	})

	var e SensorEvent
	_ = json.Unmarshal([]byte(`{"DS18B20-1":{"Temperature":31.8},"SHT3X":{"Temperature":21,"Humidity":45}}`), &e)
	if r := e.Readings(); *r.Temperature != 21 {
		t.Errorf("SHT3X should win over DS18B20, got %v", *r.Temperature)
	}
}

func TestMatterBridgeLeakSensor(t *testing.T) {
	payload := `{"Time":"2026-11-25T21:55:26","Available":true,"Leak":false,"Battery":98,"BatteryVoltage":2.843,"BatteryLevel":"OK"}`
	var e SensorEvent
	if err := json.Unmarshal([]byte(payload), &e); err != nil {
		t.Fatal(err)
	}
	if e.Available == nil || !*e.Available {
		t.Errorf("Available = %v, want true", e.Available)
	}
	if e.Leak == nil || *e.Leak {
		t.Errorf("Leak = %v, want false", e.Leak)
	}
	if len(e.Sensors) != 1 || e.Sensors[0] != model.SourceDevice {
		t.Errorf("Sensors = %v, want [%s]", e.Sensors, model.SourceDevice)
	}
	got := map[string]float64{}
	for _, r := range e.SeriesReadings() {
		if r.Source != model.SourceDevice {
			t.Errorf("reading %+v: source, want %s", r, model.SourceDevice)
		}
		got[r.Field] = r.Value
	}
	want := map[string]float64{"Leak": 0, "Battery": 98, "BatteryVoltage": 2.843}
	if len(got) != len(want) {
		t.Errorf("readings = %v, want %v", got, want)
	}
	for f, v := range want {
		if got[f] != v {
			t.Errorf("%s = %v, want %v", f, got[f], v)
		}
	}
	for f, kind := range map[string]string{"Leak": model.KindLeak, "Battery": model.KindBattery, "BatteryVoltage": model.KindBatteryVoltage} {
		if k := model.SeriesKind(model.SourceDevice, f); k != kind {
			t.Errorf("kind of %s = %s, want %s", f, k, kind)
		}
	}

	var unavailable SensorEvent
	if err := json.Unmarshal([]byte(`{"Time":"2026-11-25T21:53:23","Available":false}`), &unavailable); err != nil {
		t.Fatal(err)
	}
	if unavailable.Available == nil || *unavailable.Available || unavailable.Leak != nil || len(unavailable.Raw) != 0 {
		t.Errorf("unavailable = %+v", unavailable)
	}

	var wet SensorEvent
	if err := json.Unmarshal([]byte(`{"Time":"2026-11-25T21:55:26","Available":true,"Leak":true}`), &wet); err != nil {
		t.Fatal(err)
	}
	if wet.Leak == nil || !*wet.Leak {
		t.Errorf("Leak = %v, want true", wet.Leak)
	}
}

func TestTasmotaTopLevelStringsIgnored(t *testing.T) {
	var e SensorEvent
	if err := json.Unmarshal([]byte(`{"Time":"2026-11-20T12:00:00","Switch1":"ON","TempUnit":"C","SHT4X":{"Temperature":21,"Humidity":40}}`), &e); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.Raw[model.SourceDevice]; ok || e.Available != nil {
		t.Errorf("unexpected flat fields: %+v", e.Raw)
	}
}
