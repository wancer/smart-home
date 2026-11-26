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

// A 3-gang wall switch (Button1..3, Relay1..3, Led_i1..3, LedLink_i) is a switch that comes
// online with LWT; an energy meter on top of relays makes it a power plug.
func TestDetectSwitch(t *testing.T) {
	gpio := []int{0, 0, 320, 321, 32, 224, 0, 0, 226, 576, 0, 0, 0, 0, 0, 0, 0, 0, 225, 34, 33, 322}
	s := &DeviceState{Device: &model.Device{SensorType: model.SensorTypeOther, Template: &model.DeviceTemplate{Gpio: gpio}}}
	if !s.ApplyDetection(nil, false) || s.Device.SensorType != model.SensorTypeSwitch {
		t.Errorf("other -> %q, want switch", s.Device.SensorType)
	}
	if !s.CanToggle() || s.RelayCount() != 3 || !s.Device.AvailabilityFromLwt() {
		t.Errorf("CanToggle %v, RelayCount %d, AvailabilityFromLwt %v", s.CanToggle(), s.RelayCount(), s.Device.AvailabilityFromLwt())
	}

	var en event.SensorEvent
	if err := json.Unmarshal([]byte(`{"ENERGY":{"Power":10}}`), &en); err != nil {
		t.Fatal(err)
	}
	if !s.ApplyDetection(&en, false) || s.Device.SensorType != model.SensorTypeEnergy {
		t.Errorf("switch + ENERGY -> %q, want energy", s.Device.SensorType)
	}
}

// A leak sensor behind a Matter bridge: detected as leak, and the platform as Matter.
func TestDetectLeakSensorAndMatterPlatform(t *testing.T) {
	var e event.SensorEvent
	if err := json.Unmarshal([]byte(`{"Time":"2026-11-25T21:55:26","Available":true,"Leak":false,"Battery":98,"BatteryVoltage":2.843,"BatteryLevel":"OK"}`), &e); err != nil {
		t.Fatal(err)
	}
	if got := DetectSensorType(nil, &e); got != model.SensorTypeLeak {
		t.Errorf("type %q, want %q", got, model.SensorTypeLeak)
	}
	if !HasSensorData(model.SensorTypeLeak, &e) || !HasSensorData(model.SensorTypeMulti, &e) {
		t.Error("HasSensorData(leak/multi) = false")
	}

	state := &DeviceState{Device: &model.Device{Platform: model.PlatformTasmota, SensorType: model.SensorTypeOther}}
	if !state.DetectPlatform(&e) || state.Device.Platform != model.PlatformMatter {
		t.Errorf("platform %q, want %q", state.Device.Platform, model.PlatformMatter)
	}
	if !state.ApplyDetection(&e, false) || state.Device.SensorType != model.SensorTypeLeak {
		t.Errorf("type %q, want %q", state.Device.SensorType, model.SensorTypeLeak)
	}
	if state.Device.IsTasmota() || state.Device.AvailabilityFromLwt() {
		t.Error("a Matter device takes commands or comes online with the bridge's LWT")
	}

	// a device known to run Tasmota stays Tasmota
	tasmota := &DeviceState{Device: &model.Device{Platform: model.PlatformTasmota, FirmwareVersion: "14.3.0(tasmota)"}}
	if tasmota.DetectPlatform(&e) {
		t.Error("Tasmota device switched to Matter")
	}
	// no Available: no detection
	var plain event.SensorEvent
	if err := json.Unmarshal([]byte(`{"SHT4X":{"Temperature":21,"Humidity":40}}`), &plain); err != nil {
		t.Fatal(err)
	}
	if (&DeviceState{Device: &model.Device{Platform: model.PlatformTasmota}}).DetectPlatform(&plain) {
		t.Error("detected Matter without Available")
	}
}

func TestDetectContactSensor(t *testing.T) {
	var e event.SensorEvent
	if err := json.Unmarshal([]byte(`{"Available":true,"Contact":false,"Battery":90}`), &e); err != nil {
		t.Fatal(err)
	}
	if got := DetectSensorType(nil, &e); got != model.SensorTypeContact {
		t.Errorf("type %q, want %q", got, model.SensorTypeContact)
	}
}

// A bridged device is unavailable on "Available":false, or while its bridge is offline.
func TestUnavailable(t *testing.T) {
	state := &DeviceState{Device: &model.Device{Platform: model.PlatformMatter}}
	yes, no := true, false
	if state.Unavailable(&event.SensorEvent{Available: &yes}) {
		t.Error("available device is unavailable")
	}
	if !state.Unavailable(&event.SensorEvent{Available: &no}) {
		t.Error("Available:false is available")
	}
	state.BridgeOffline = true
	if !state.Unavailable(&event.SensorEvent{Available: &yes}) {
		t.Error("retained Available:true with the bridge offline is available")
	}
}

// One command in flight; a result reaches only a waiting command.
func TestCommandWait(t *testing.T) {
	state := &DeviceState{Device: &model.Device{Platform: model.PlatformMatter}}
	if state.CommandResult(event.CommandResult{Command: "Identify", Result: "Done"}) {
		t.Error("result delivered without a waiting command")
	}
	wait, err := state.BeginCommand()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.BeginCommand(); err != ErrCommandPending {
		t.Errorf("second command: err %v, want ErrCommandPending", err)
	}
	if !state.CommandResult(event.CommandResult{Command: "Identify", Result: "Done"}) {
		t.Error("result not delivered")
	}
	if r := <-wait; r.Result != "Done" {
		t.Errorf("result %q", r.Result)
	}
	state.EndCommand()
	if _, err := state.BeginCommand(); err != nil {
		t.Errorf("after EndCommand: %v", err)
	}
}
