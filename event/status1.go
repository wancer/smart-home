package event

// Status1 is Tasmota's STATUS1 reply. Only the fields in use are parsed.
type Status1 struct {
	StatusPRM *StatusPRM `json:"StatusPRM"`
}

type StatusPRM struct {
	RestartReason string `json:"RestartReason"` // reason of the last (re)boot, e.g. "Power On", "Software/System restart"
	BootCount     *int   `json:"BootCount"`
}
