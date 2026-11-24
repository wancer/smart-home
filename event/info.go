package event

// Info is one of Tasmota's boot messages, tele/<topic>/INFO1 ({"Info1":{...}}) or INFO3
// ({"Info3":{...}}). ESPHome devices publish them from their YAML on connect; Tasmota devices
// get the same data polled (STATUS2, STATUS1). Only the fields in use are parsed.
type Info struct {
	Info1 *Info1 `json:"Info1"`
	Info3 *Info3 `json:"Info3"`
}

type Info1 struct {
	Version string `json:"Version"` // e.g. "2026.11.0(esphome)"
}

type Info3 struct {
	RestartReason string `json:"RestartReason"` // reason of the last boot, e.g. "Power on"
}
