package internal

import (
	"encoding/json"
	"smart-home/event"
	"smart-home/model"
	"testing"
)

func TestDetectSensorTypeDS18B20(t *testing.T) {
	cases := []struct{ payload, want string }{
		{`{"DS18B20":{"Id":"0B257989FB50","Temperature":31.8}}`, model.SensorTypeTempHumid},
		{`{"DS18B20":{"Temperature":31.8},"ESP32":{"Temperature":50}}`, model.SensorTypeTempHumid},
		{`{"ESP32":{"Temperature":50}}`, model.SensorTypeOther},
		{`{"ENERGY":{"Power":10},"DS18B20":{"Temperature":31.8}}`, model.SensorTypeMulti},
	}
	for _, c := range cases {
		var e event.SensorEvent
		if err := json.Unmarshal([]byte(c.payload), &e); err != nil {
			t.Fatal(err)
		}
		if got := DetectSensorType(nil, &e); got != c.want {
			t.Errorf("%s: type %q, want %q", c.payload, got, c.want)
		}
		if !HasSensorData(c.want, &e) && c.want != model.SensorTypeOther {
			t.Errorf("%s: HasSensorData(%q) = false", c.payload, c.want)
		}
	}
}
