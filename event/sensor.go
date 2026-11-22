package event

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
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

// Barometer is the pressure of a BME280 / BMP280 / ... (its temperature and humidity go to TempHum).
type Barometer struct {
	Pressure float32 `json:"Pressure"` // hPa
}

// DcMonitor is a DC voltage / current / power monitor (INA219, INA226, ...).
type DcMonitor struct {
	Voltage float32 `json:"Voltage"` // V
	Current float32 `json:"Current"` // A
	Power   float32 `json:"Power"`   // W
}

// GasSensor is an MQ-xxx gas sensor on the ADC (MQ-2, MQ-135, ...), reported in the ANALOG
// object as "MQ135_1": ppm (as calibrated in Tasmota).
type GasSensor struct {
	Name string  // e.g. MQ135_1
	Ppm  float32 // ppm
}

// Chip is the ESP32 internal temperature sensor, reported with SetOption146 1.
type Chip struct {
	Temperature float32 `json:"Temperature"`
}

// SensorEvent is a tele/SENSOR (or STATUS10 StatusSNS) payload. Sensors are recognised by their
// fields, not their names: the first one with CarbonDioxide is Co2 (SCD40, MHZ19B, ...), the
// first other one with Temperature and Humidity is TempHum (SHT4X, SHT3X, AM2301, BME280, ...),
// the first one with Pressure is Baro (BME280, BMP280, ...) and the first one with Voltage and
// Current is Dc (INA219, ...). The first MQ-xxx value in ANALOG is Gas.
type SensorEvent struct {
	Time    TasmotaTime
	Energy  *Energy
	Co2     *Co2
	TempHum *TempHumidity
	Baro    *Barometer
	Dc      *DcMonitor
	Gas     *GasSensor
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
		case "ANALOG":
			e.Gas = parseGas(value)
			continue
		}

		var probe struct {
			CarbonDioxide *uint
			Temperature   *float32
			Humidity      *float32
			DewPoint      *float32
			Pressure      *float32
			Voltage       *float32
			Current       *float32
			Power         *float32
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
		if probe.Pressure != nil && e.Baro == nil {
			e.Baro = &Barometer{Pressure: *probe.Pressure}
		}
		if probe.Voltage != nil && probe.Current != nil && e.Dc == nil {
			e.Dc = &DcMonitor{Voltage: *probe.Voltage, Current: *probe.Current, Power: deref(probe.Power)}
		}
	}
	return nil
}

// parseGas returns the first MQ-xxx value (by name) of an ANALOG object, nil if there is none
// (ANALOG also carries raw A1, Temperature1, Range1, ...).
func parseGas(value json.RawMessage) *GasSensor {
	var analog map[string]json.RawMessage
	if err := json.Unmarshal(value, &analog); err != nil {
		return nil
	}
	for _, name := range slices.Sorted(maps.Keys(analog)) {
		if !strings.HasPrefix(name, "MQ") {
			continue
		}
		var ppm float32
		if err := json.Unmarshal(analog[name], &ppm); err != nil {
			continue
		}
		return &GasSensor{Name: name, Ppm: ppm}
	}
	return nil
}

func deref(v *float32) float32 {
	if v == nil {
		return 0
	}
	return *v
}

// Readings are the values of every known sensor in a message, whatever the device's
// sensor type; nil = not reported. Temperature / humidity come from TempHum (a dedicated
// T-H sensor, including BME280), else from the CO2 sensor.
type Readings struct {
	// AC energy meter
	Period        *uint
	Power         *uint
	ApparentPower *uint
	ReactivePower *uint
	Voltage       *uint
	Current       *float32
	Today         *float32 // kWh
	Yesterday     *float32 // kWh

	CO2         *uint
	Temperature *float32
	Humidity    *float32
	DewPoint    *float32
	Pressure    *float32 // hPa
	Gas         *float32 // ppm, MQ-xxx gas sensor

	// DC power monitor
	DcVoltage *float32
	DcCurrent *float32
	DcPower   *float32
}

func (e *SensorEvent) Readings() Readings {
	var r Readings
	if e == nil {
		return r
	}
	if en := e.Energy; en != nil {
		r.Period, r.Power = new(en.Period), new(en.Power)
		r.ApparentPower, r.ReactivePower = new(en.ApparentPower), new(en.ReactivePower)
		r.Voltage, r.Current = new(en.Voltage), new(en.Current)
		r.Today, r.Yesterday = new(en.Today), new(en.Yesterday)
	}
	if b := e.Baro; b != nil {
		r.Pressure = new(b.Pressure)
	}
	if g := e.Gas; g != nil {
		r.Gas = new(g.Ppm)
	}
	if i := e.Dc; i != nil {
		r.DcVoltage, r.DcCurrent, r.DcPower = new(i.Voltage), new(i.Current), new(i.Power)
	}
	if c := e.Co2; c != nil {
		r.CO2 = new(c.CO2)
	}

	// climate, in order of precedence
	switch {
	case e.TempHum != nil:
		r.Temperature, r.Humidity, r.DewPoint = new(e.TempHum.Temperature), new(e.TempHum.Humidity), new(e.TempHum.DewPoint)
	case e.Co2 != nil:
		r.Temperature, r.Humidity, r.DewPoint = new(e.Co2.Temperature), new(e.Co2.Humidity), new(e.Co2.DewPoint)
	}
	return r
}

// ChipTemperature returns the ESP32 chip temperature, nil if the device doesn't report it.
func (e *SensorEvent) ChipTemperature() *float32 {
	if e == nil || e.Chip == nil {
		return nil
	}
	return new(e.Chip.Temperature)
}
