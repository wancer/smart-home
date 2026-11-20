package event

import "time"

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

// BME280 is a temperature / humidity / barometric pressure sensor.
type BME280 struct {
	Temperature float32 `json:"Temperature"`
	Humidity    float32 `json:"Humidity"`
	DewPoint    float32 `json:"DewPoint"`
	Pressure    float32 `json:"Pressure"` // hPa
}

// INA219 is a DC voltage / current / power monitor.
type INA219 struct {
	Voltage float32 `json:"Voltage"` // V
	Current float32 `json:"Current"` // A
	Power   float32 `json:"Power"`   // W
}

// Chip is the ESP32 internal temperature sensor, reported with SetOption146 1.
type Chip struct {
	Temperature float32 `json:"Temperature"`
}

type SensorEvent struct {
	Time    TasmotaTime   `json:"Time"`
	Energy  *Energy       `json:"ENERGY"`
	Co2     *Co2          `json:"SCD40"`
	TempHum *TempHumidity `json:"SHT4X"`
	BME280  *BME280       `json:"BME280"`
	INA219  *INA219       `json:"INA219"`
	Chip    *Chip         `json:"ESP32"`
}

// Readings are the values of every known sensor in a message, whatever the device's
// sensor type; nil = not reported. When several sensors report the same quantity, the
// dedicated one wins: SHT4X, then BME280, then SCD40.
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
	if b := e.BME280; b != nil {
		r.Pressure = new(b.Pressure)
	}
	if i := e.INA219; i != nil {
		r.DcVoltage, r.DcCurrent, r.DcPower = new(i.Voltage), new(i.Current), new(i.Power)
	}
	if c := e.Co2; c != nil {
		r.CO2 = new(c.CO2)
	}

	// climate, in order of precedence
	switch {
	case e.TempHum != nil:
		r.Temperature, r.Humidity, r.DewPoint = new(e.TempHum.Temperature), new(e.TempHum.Humidity), new(e.TempHum.DewPoint)
	case e.BME280 != nil:
		r.Temperature, r.Humidity, r.DewPoint = new(e.BME280.Temperature), new(e.BME280.Humidity), new(e.BME280.DewPoint)
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
