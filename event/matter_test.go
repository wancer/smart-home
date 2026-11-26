package event

import (
	"encoding/json"
	"testing"
)

func TestMatterInfoHardware(t *testing.T) {
	var i MatterInfo
	if err := json.Unmarshal([]byte(`{"VendorName":"IKEA of Sweden","ProductName":"KLIPPBOK","NodeLabel":"","HardwareVersion":"1","SoftwareVersion":"1.0.0","SerialNumber":"ABC123"}`), &i); err != nil {
		t.Fatal(err)
	}
	if got, want := i.Hardware(), "IKEA of Sweden KLIPPBOK, HW 1, SN ABC123"; got != want {
		t.Errorf("Hardware() = %q, want %q", got, want)
	}
	if got := (&MatterInfo{}).Hardware(); got != "" {
		t.Errorf("empty info: Hardware() = %q", got)
	}
}

func TestCommandResult(t *testing.T) {
	cases := []struct {
		payload         string
		command, result string
		ok              bool
	}{
		{`{"Identify":"Done"}`, "Identify", CommandDone, true},
		{`{"Identify":"Busy"}`, "Identify", CommandBusy, true},
		{`{"Command":"Unknown"}`, "", CommandUnknown, true},
		{`{"Identify":1}`, "", "", false},
		{`{"A":"x","B":"y"}`, "", "", false},
	}
	for _, c := range cases {
		var raw map[string]any
		if err := json.Unmarshal([]byte(c.payload), &raw); err != nil {
			t.Fatal(err)
		}
		r, ok := NewCommandResult(raw)
		if ok != c.ok || (ok && (r.Command != c.command || r.Result != c.result)) {
			t.Errorf("%s: %+v %v", c.payload, r, ok)
		}
	}
}

func TestContactSensor(t *testing.T) {
	var e SensorEvent
	if err := json.Unmarshal([]byte(`{"Available":true,"Contact":true,"Battery":90}`), &e); err != nil {
		t.Fatal(err)
	}
	if e.Contact == nil || !*e.Contact || e.Leak != nil {
		t.Errorf("Contact = %v, Leak = %v", e.Contact, e.Leak)
	}
	// a Matter sensor that hasn't reported yet: unknown, not false
	var empty SensorEvent
	if err := json.Unmarshal([]byte(`{"Available":true}`), &empty); err != nil {
		t.Fatal(err)
	}
	if empty.Contact != nil || empty.Leak != nil || len(empty.SeriesReadings()) != 0 {
		t.Errorf("unexpected values: %+v", empty)
	}
}
