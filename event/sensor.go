package event

import (
	"encoding/json"
	"maps"
	"slices"
	"time"
)

type TasmotaTime time.Time

func (d *TasmotaTime) UnmarshalJSON(in []byte) error {
	// Strip the double quotes from the JSON string.
	in = in[1 : len(in)-1]
	// Replace "T" to " ": 2022-01-02T12:34:56 -> 2022-01-02 12:34:56
	in[10] = byte(' ')
	// Parse the result using our desired format.
	parsed, err := time.Parse(time.DateTime, string(in))
	if err != nil {
		return err
	}
	// finally, assign *d
	*d = TasmotaTime(parsed)
	return nil
}

type Energy struct {
	TotalStartTime TasmotaTime `json:"TotalStartTime"` // DateTime of calculation for Total
	Total          float32     `json:"Total"`          // Total Energy usage including Today
	Yesterday      float32     `json:"Yesterday"`      // Total Energy usage between 00:00 and 24:00 yesterday
	Today          float32     `json:"Today"`          // Total Energy usage today from 00:00 until now
	Period         uint        `json:"Period"`         // Energy usage between previous message and now
	Power          uint        `json:"Power"`          // Current effective power load
	ApparentPower  uint        `json:"ApparentPower"`  // Power load on the cable = sqrt(Power^2 + ReactivePower^2)
	ReactivePower  uint        `json:"ReactivePower"`  // Reactive load
	Factor         float32     `json:"Factor"`         // The ratio of the real power flowing to the load to the apparent power in the circuit
	Voltage        uint        `json:"Voltage"`        // Current line voltage
	Current        float32     `json:"Current"`        // Current line current
}

type TempHumidity struct {
	Temperature float32 `json:"Temperature"`
	Humidity    float32 `json:"Humidity"`
	DewPoint    float32 `json:"DewPoint"`
}

type Co2 struct {
	CO2         uint    `json:"CarbonDioxide"`
	Temperature float32 `json:"Temperature"`
	Humidity    float32 `json:"Humidity"`
	DewPoint    float32 `json:"DewPoint"`
}

// Chip is the ESP32 internal temperature sensor, reported with SetOption146 1.
type Chip struct {
	Temperature float32 `json:"Temperature"`
}

// SensorEvent is a tele/SENSOR (or STATUS10 StatusSNS) payload. Sensors are recognised by their
// fields, not their names: the first one with CarbonDioxide is Co2 (SCD40, MHZ19B, ...), the
// first other one with Temperature and Humidity is TempHum (SHT4X, SHT3X, AM2301, BME280, ...).
type SensorEvent struct {
	Time    TasmotaTime
	Energy  *Energy
	Co2     *Co2
	TempHum *TempHumidity
	Chip    *Chip

	Sensors []string // names of all sensor objects, sorted (ENERGY, SCD40, ESP32, ...)
}

func (e *SensorEvent) UnmarshalJSON(in []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(in, &raw); err != nil {
		return err
	}
	*e = SensorEvent{}
	if t, ok := raw["Time"]; ok {
		if err := json.Unmarshal(t, &e.Time); err != nil {
			return err
		}
	}

	names := slices.Sorted(maps.Keys(raw))
	for _, name := range names {
		value := raw[name]
		if len(value) == 0 || value[0] != '{' { // Time, TempUnit, ...
			continue
		}
		e.Sensors = append(e.Sensors, name)

		switch name {
		case "ENERGY":
			if err := json.Unmarshal(value, &e.Energy); err != nil {
				return err
			}
			continue
		case "ESP32":
			if err := json.Unmarshal(value, &e.Chip); err != nil {
				return err
			}
			continue
		}

		var probe struct {
			CarbonDioxide *uint
			Temperature   *float32
			Humidity      *float32
			DewPoint      *float32
		}
		if err := json.Unmarshal(value, &probe); err != nil {
			continue // not a sensor we know
		}
		switch {
		case probe.CarbonDioxide != nil && e.Co2 == nil:
			e.Co2 = &Co2{CO2: *probe.CarbonDioxide}
			e.Co2.Temperature, e.Co2.Humidity, e.Co2.DewPoint = deref(probe.Temperature), deref(probe.Humidity), deref(probe.DewPoint)
		case probe.CarbonDioxide == nil && probe.Temperature != nil && probe.Humidity != nil && e.TempHum == nil:
			e.TempHum = &TempHumidity{Temperature: *probe.Temperature, Humidity: *probe.Humidity, DewPoint: deref(probe.DewPoint)}
		}
	}
	return nil
}

func deref(v *float32) float32 {
	if v == nil {
		return 0
	}
	return *v
}

// ChipTemperature returns the ESP32 chip temperature, nil if the device doesn't report it.
func (e *SensorEvent) ChipTemperature() *float32 {
	if e == nil || e.Chip == nil {
		return nil
	}
	return new(e.Chip.Temperature)
}
