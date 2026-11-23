package event

import (
	"cmp"
	"encoding/json"
	"maps"
	"math"
	"slices"
	"strconv"
	"time"

	"smart-home/model"
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

// MqReading is an MQ-xxx gas sensor reading, computed by the backend from the raw analog value
// (internal.DeviceState.ComputeMq) for devices with the sensor set up; nil otherwise. The sensor
// reacts to many gases (NH3, NOx, benzene, alcohol, smoke, ...) and can't tell them apart, so
// only Ratio is a metric, never a CO2 value.
type MqReading struct {
	Raw   float32  // ADC value
	Volts float32  // module output, V
	Vcc   float32  // module supply, V
	VccDc bool     // Vcc measured by the DC monitor (INA219), not the configured supply
	Rs    float32  // sensor resistance, Ω (not temperature/humidity corrected)
	RsAvg float32  // corrected and averaged over the last minute, Ω
	Ratio *float32 // R0 / RsAvg: 1.0 = the calibration air, higher = more gas; nil = not calibrated
}

// LightMeter is an ambient light sensor (BH1750, TSL2561, VEML7700, ...).
type LightMeter struct {
	Illuminance float32 `json:"Illuminance"` // lx
}

// ColorSensor is an RGB colour sensor (TCS34725, ...). Illuminance and ColorTemperature are nil
// when it doesn't report them.
type ColorSensor struct {
	Red, Green, Blue, Clear float32
	Illuminance             *float32 // lx
	ColorTemperature        *float32 // K
}

// Chip is the ESP32 internal temperature sensor, reported with SetOption146 1.
type Chip struct {
	Temperature float32 `json:"Temperature"`
}

// SensorEvent is a tele/SENSOR (or STATUS10 StatusSNS) payload. Sensors are recognised by their
// fields, not their names: the first one with CarbonDioxide is Co2 (SCD40, MHZ19B, ...), the
// first other one with Temperature and Humidity is TempHum (SHT4X, SHT3X, AM2301, BME280, ...),
// the first one with Pressure is Baro (BME280, BMP280, ...) and the first one with Voltage and
// Current is Dc (INA219, ...), the first one with Red, Green and Blue is Color (TCS34725, ...)
// and the first other one with Illuminance is Light (BH1750, ...). ANALOG values are kept raw in Analog.
type SensorEvent struct {
	Time    TasmotaTime
	Energy  *Energy
	Co2     *Co2
	TempHum *TempHumidity
	Baro    *Barometer
	Dc      *DcMonitor
	Light   *LightMeter
	Color   *ColorSensor
	Chip    *Chip

	Analog map[string]float32 // raw ANALOG values (A1, ...)
	Mq     *MqReading         // set by the backend from Analog, see MqReading

	Sensors []string // names of all sensor objects, sorted (ENERGY, SCD40, ESP32, ...)

	// Raw is every numeric field of every sensor object as reported (sensor -> field -> value),
	// including the ones no metric uses (TCS34725 Red/Green/Blue/Clear, ...). Live only, never stored.
	Raw map[string]map[string]float64
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
		if fields := parseRaw(value); len(fields) > 0 {
			if e.Raw == nil {
				e.Raw = map[string]map[string]float64{}
			}
			e.Raw[name] = fields
		}

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
			e.Analog = parseAnalog(value)
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
			Illuminance   *float32
			Red           *float32
			Green         *float32
			Blue          *float32
			Clear         *float32
			CCT           *float32 `json:"ColorTemperature"`
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
		isColor := probe.Red != nil && probe.Green != nil && probe.Blue != nil
		switch {
		case isColor && e.Color == nil:
			e.Color = &ColorSensor{
				Red: *probe.Red, Green: *probe.Green, Blue: *probe.Blue, Clear: deref(probe.Clear),
				Illuminance: probe.Illuminance, ColorTemperature: probe.CCT,
			}
		case !isColor && probe.Illuminance != nil && e.Light == nil:
			e.Light = &LightMeter{Illuminance: *probe.Illuminance}
		}
	}
	return nil
}

// parseAnalog returns the numeric values of the ANALOG object (A1, Temperature1, Range1, ...).
func parseAnalog(value json.RawMessage) map[string]float32 {
	var raw map[string]any
	if err := json.Unmarshal(value, &raw); err != nil {
		return nil
	}
	values := map[string]float32{}
	for k, v := range raw {
		if f, ok := v.(float64); ok {
			values[k] = float32(f)
		}
	}
	return values
}

// parseRaw returns the numeric fields of a sensor object. Arrays of numbers (multi-phase
// ENERGY, ...) become Field1, Field2, ...; strings and nested objects are skipped.
func parseRaw(value json.RawMessage) map[string]float64 {
	var raw map[string]any
	if err := json.Unmarshal(value, &raw); err != nil {
		return nil
	}
	fields := map[string]float64{}
	for k, v := range raw {
		switch v := v.(type) {
		case float64:
			fields[k] = v
		case []any:
			for i, item := range v {
				if f, ok := item.(float64); ok {
					fields[k+strconv.Itoa(i+1)] = f
				}
			}
		}
	}
	return fields
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
	GasRatio    *float32 // MQ-xxx gas level vs. calibration air (1.0 = baseline)
	GasRs       *float32 // MQ-xxx sensor resistance, Ω, uncorrected; only stored (to recompute history)
	Illuminance *float32 // lx: the light meter (BH1750), else the colour sensor (TCS34725)
	ColorTemp   *float32 // K, colour sensor

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
	if m := e.Mq; m != nil {
		r.GasRs = new(m.Rs)
		if m.Ratio != nil {
			r.GasRatio = new(*m.Ratio)
		}
	}
	if i := e.Dc; i != nil {
		r.DcVoltage, r.DcCurrent, r.DcPower = new(i.Voltage), new(i.Current), new(i.Power)
	}
	if c := e.Co2; c != nil {
		r.CO2 = new(c.CO2)
	}
	// a dedicated light meter measures lux better than a colour sensor
	switch {
	case e.Light != nil:
		r.Illuminance = new(e.Light.Illuminance)
	case e.Color != nil && e.Color.Illuminance != nil:
		r.Illuminance = new(*e.Color.Illuminance)
	}
	if e.Color != nil && e.Color.ColorTemperature != nil {
		r.ColorTemp = new(*e.Color.ColorTemperature)
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

// Reading is one numeric value of a message: Source is the sensor object (ENERGY, BH1750, ESP32,
// ANALOG, ...) or a computed source (MQ), Field its field (arrays as Field1, Field2, ...). One
// series per device × Source × Field; its kind is model.SeriesKind(Source, Field).
type Reading struct {
	Source string
	Field  string
	Value  float64
}

// SeriesReadings returns every measurement of the message: one per Raw entry not skipped by
// model.SkipSeriesField, the ANALOG values from Analog when Raw has none for ANALOG, and the
// computed MQ Ratio (when calibrated) and Rs. NaN/Inf values are dropped. Sorted by Source, then
// Field; nil for a nil event.
func (e *SensorEvent) SeriesReadings() []Reading {
	if e == nil {
		return nil
	}
	var out []Reading
	add := func(source, field string, value float64) {
		if math.IsNaN(value) || math.IsInf(value, 0) || model.SkipSeriesField(source, field) {
			return
		}
		out = append(out, Reading{Source: source, Field: field, Value: value})
	}
	for source, fields := range e.Raw {
		for field, value := range fields {
			add(source, field, value)
		}
	}
	if _, ok := e.Raw["ANALOG"]; !ok {
		for field, value := range e.Analog {
			add("ANALOG", field, float64(value))
		}
	}
	if m := e.Mq; m != nil {
		add(model.SourceMq, "Rs", math.Round(float64(m.Rs)*10) / 10)
		if m.Ratio != nil {
			add(model.SourceMq, "Ratio", math.Round(float64(*m.Ratio)*100) / 100)
		}
	}
	slices.SortFunc(out, func(a, b Reading) int {
		if c := cmp.Compare(a.Source, b.Source); c != 0 {
			return c
		}
		return cmp.Compare(a.Field, b.Field)
	})
	return out
}
