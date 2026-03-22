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

type SensorData struct {
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

type SensorEvent struct {
	Time   TasmotaTime `json:"Time"`
	Energy SensorData  `json:"ENERGY"`
}
