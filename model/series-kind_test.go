package model

import "testing"

func TestSeriesKind(t *testing.T) {
	cases := []struct {
		source, field, want string
	}{
		{"ENERGY", "Power", KindPower},
		{"ENERGY", "ApparentPower", KindApparentPower},
		{"ENERGY", "ReactivePower", KindReactivePower},
		{"ENERGY", "Factor", KindPowerFactor},
		{"ENERGY", "Voltage", KindVoltage},
		{"ENERGY", "Current", KindCurrent},
		{"ENERGY", "Frequency", KindRaw},
		{"INA219", "Voltage", KindDcVoltage},
		{"INA219", "Current", KindDcCurrent},
		{"INA219", "Power", KindDcPower},
		{"ESP32", "Temperature", KindChipTemperature},
		{"BME280", "Temperature", KindTemperature},
		{"BME280", "Humidity", KindHumidity},
		{"BME280", "DewPoint", KindDewPoint},
		{"BME280", "Pressure", KindPressure},
		{"SCD40", "CarbonDioxide", KindCo2},
		{"SCD40", "Temperature", KindTemperature},
		{"BH1750", "Illuminance", KindIlluminance},
		{"TCS34725", "Illuminance", KindIlluminance},
		{"TCS34725", "ColorTemperature", KindColorTemp},
		{"TCS34725", "Red", KindColorRed},
		{"TCS34725", "Green", KindColorGreen},
		{"TCS34725", "Blue", KindColorBlue},
		{"TCS34725", "Clear", KindColorClear},
		{"INA219", "BusVoltage", KindDcBusVoltage},
		{"INA219", "ShuntVoltage", KindDcShuntVoltage},
		{"MQ", "Ratio", KindGasRatio},
		{"MQ", "Rs", KindRaw},
		{"ANALOG", "A1", KindRaw},
		{"ANALOG", "Range1", KindRaw},
		{"SomeSensor", "Ratio", KindRaw}, // gas_ratio only from MQ

		// numbered fields (arrays in the payload)
		{"ENERGY", "Voltage1", KindVoltage},
		{"ENERGY", "Voltage2", KindVoltage},
		{"ENERGY", "Power3", KindPower},
		{"ENERGY", "Factor1", KindPowerFactor},
		{"ANALOG", "Temperature1", KindTemperature},
		{"ANALOG", "Illuminance1", KindIlluminance},
		{"ESP32", "Temperature1", KindChipTemperature},
		{"PZEM", "Voltage1", KindRaw}, // numbered V/A/W of a non-ENERGY sensor: AC inputs, not DC
		{"ANALOG", "Power1", KindRaw},
		{"X", "123", KindRaw},
	}
	for _, c := range cases {
		if got := SeriesKind(c.source, c.field); got != c.want {
			t.Errorf("SeriesKind(%q, %q) = %q, want %q", c.source, c.field, got, c.want)
		}
		if _, ok := Kinds[c.want]; !ok {
			t.Errorf("kind %q missing from Kinds", c.want)
		}
	}
}

func TestKindsTable(t *testing.T) {
	all := []string{
		KindPower, KindApparentPower, KindReactivePower, KindPowerFactor, KindVoltage, KindCurrent,
		KindDcVoltage, KindDcCurrent, KindDcPower, KindTemperature, KindChipTemperature,
		KindHumidity, KindDewPoint, KindCo2, KindPressure, KindIlluminance, KindColorTemp,
		KindGasRatio, KindRaw, KindDcBusVoltage, KindDcShuntVoltage,
		KindColorRed, KindColorGreen, KindColorBlue, KindColorClear,
		KindLeak, KindContact, KindBattery, KindBatteryVoltage,
	}
	if len(Kinds) != len(all) {
		t.Errorf("Kinds has %d entries, want %d", len(Kinds), len(all))
	}
	for _, k := range all {
		info, ok := Kinds[k]
		if !ok {
			t.Errorf("kind %q missing", k)
			continue
		}
		if info.Kind != k {
			t.Errorf("Kinds[%q].Kind = %q", k, info.Kind)
		}
	}
	if Kinds[KindTemperature].Unit != "°C" || Kinds[KindPressure].Unit != "hPa" || Kinds[KindGasRatio].Unit != "" {
		t.Errorf("unexpected units: %+v", Kinds)
	}
}

func TestSkipSeriesField(t *testing.T) {
	skip := [][2]string{
		{"ENERGY", "Total"}, {"ENERGY", "Today"}, {"ENERGY", "Yesterday"}, {"ENERGY", "Period"},
		{"ENERGY", "TotalStartTime"}, {"ENERGY", "Today1"}, {"ENERGY", "Total2"},
		{"ENERGY", "ImportActive"}, {"ENERGY", "ExportActive"},
		{"INA219", "Id"}, {"SHT3X", "Id"},
	}
	for _, s := range skip {
		if !SkipSeriesField(s[0], s[1]) {
			t.Errorf("%s.%s must be skipped", s[0], s[1])
		}
	}
	keep := [][2]string{
		{"ENERGY", "Power"}, {"ENERGY", "Voltage1"}, {"ENERGY", "Frequency"},
		{"INA219", "Voltage"}, {"BME280", "Temperature"}, {"Counter", "Total"},
		{"ANALOG", "A1"}, {"MQ", "Ratio"}, {"MQ", "Rs"},
	}
	for _, k := range keep {
		if SkipSeriesField(k[0], k[1]) {
			t.Errorf("%s.%s must be kept", k[0], k[1])
		}
	}
}

func TestIsLiveOnly(t *testing.T) {
	live := [][2]string{
		{"ENERGY", "ApparentPower"}, {"ENERGY", "ReactivePower"}, {"ENERGY", "Factor"}, {"ENERGY", "Factor2"},
		{"SHT3X", "DewPoint"}, {"INA219", "BusVoltage"}, {"INA219", "ShuntVoltage"},
		{"TCS34725", "Red"}, {"TCS34725", "Green"}, {"TCS34725", "Blue"}, {"TCS34725", "Clear"},
	}
	for _, l := range live {
		if !IsLiveOnly(SeriesKind(l[0], l[1])) {
			t.Errorf("%s.%s must be live-only", l[0], l[1])
		}
	}
	keep := [][2]string{
		{"ENERGY", "Power"}, {"ENERGY", "Voltage"}, {"ENERGY", "Current"}, {"SHT3X", "Temperature"},
		{"SHT3X", "Humidity"}, {"ESP32", "Temperature"}, {"INA219", "Voltage"}, {"INA219", "Current"},
		{"TCS34725", "Illuminance"}, {"TCS34725", "ColorTemperature"}, {"ANALOG", "A1"}, {"MQ", "Rs"},
	}
	for _, k := range keep {
		if IsLiveOnly(SeriesKind(k[0], k[1])) {
			t.Errorf("%s.%s must keep history", k[0], k[1])
		}
	}
}
