package event

// TeleState is Tasmota's tele/<topic>/STATE payload (also returned by STATUS11 as StatusSTS).
// Only the fields in use are parsed.
type TeleState struct {
	UptimeSec *uint64 `json:"UptimeSec"`
	LoadAvg   *uint   `json:"LoadAvg"`
}

type Status11 struct {
	StatusSTS *TeleState `json:"StatusSTS"`
}
