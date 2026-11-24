package event

import (
	"encoding/json"
	"testing"
)

func TestInfo(t *testing.T) {
	var i1 Info
	if err := json.Unmarshal([]byte(`{"Info1":{"Module":"esp32dev","Version":"2026.11.0(esphome)"}}`), &i1); err != nil {
		t.Fatal(err)
	}
	if i1.Info1 == nil || i1.Info1.Version != "2026.11.0(esphome)" || i1.Info3 != nil {
		t.Errorf("INFO1 parsed as %+v", i1)
	}

	var i3 Info
	if err := json.Unmarshal([]byte(`{"Info3":{"RestartReason":"Power on","BootCount":3}}`), &i3); err != nil {
		t.Fatal(err)
	}
	if i3.Info3 == nil || i3.Info3.RestartReason != "Power on" || i3.Info1 != nil {
		t.Errorf("INFO3 parsed as %+v", i3)
	}
}
