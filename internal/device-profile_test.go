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

// An RGB controller without a relay (Button1 + PWM1..3) is a light that can be toggled.
func TestDetectLightController(t *testing.T) {
	gpio := []int{32, 0, 0, 0, 417, 0, 0, 0, 416, 0, 418, 0, 0, 0}
	if got := DetectSensorType(gpio, nil); got != model.SensorTypeLight {
		t.Errorf("type %q, want %q", got, model.SensorTypeLight)
	}
	// chip temperature telemetry doesn't change it
	var e event.SensorEvent
	if err := json.Unmarshal([]byte(`{"ESP32":{"Temperature":50}}`), &e); err != nil {
		t.Fatal(err)
	}
	if got := DetectSensorType(gpio, &e); got != model.SensorTypeLight {
		t.Errorf("with chip temperature: type %q, want %q", got, model.SensorTypeLight)
	}

	s := &DeviceState{Device: &model.Device{SensorType: model.SensorTypeOther, Template: &model.DeviceTemplate{Gpio: gpio}}}
	if !s.ApplyDetection(nil, false) || s.Device.SensorType != model.SensorTypeLight {
		t.Errorf("other -> %q, want light", s.Device.SensorType)
	}
	if !s.CanToggle() || !s.HasLight() || s.LightPowerIndex() != 1 {
		t.Errorf("CanToggle %v, HasLight %v, LightPowerIndex %d", s.CanToggle(), s.HasLight(), s.LightPowerIndex())
	}
	if !s.Device.AvailabilityFromLwt() {
		t.Error("a light must come online with LWT")
	}

	// a relay + energy meter plug with a PWM LED stays a power plug
	plug := []int{224, 416, 0}
	var en event.SensorEvent
	if err := json.Unmarshal([]byte(`{"ENERGY":{"Power":10}}`), &en); err != nil {
		t.Fatal(err)
	}
	if got := DetectSensorType(plug, &en); got != model.SensorTypeEnergy {
		t.Errorf("plug: type %q, want energy", got)
	}
}
