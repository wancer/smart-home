package event

import (
	"strconv"
	"strings"
)

// Light is a Tasmota light state from stat/RESULT, e.g. the reply to Dimmer / HSBColor / Color:
// {"POWER":"ON","Dimmer":100,"Color":"FF0000","HSBColor":"0,100,100","Channel":[100,0,0]}
// The light is POWER<n> with n = number of relays + 1 (plain "POWER" when there is no relay).
type Light struct {
	PowerIndex int   // index of the POWER<n> key; 0 if the payload has none
	Power      *bool // value of that key
	Dimmer     *uint // 0..100
	Hue        *uint // 0..360
	Saturation *uint // 0..100
}

// NewLight returns false if the payload is not a light state. A lone "POWER"/"POWER1"
// is left to the regular Power event.
func NewLight(raw map[string]any) (*Light, bool) {
	l := &Light{}
	found := false

	if v, ok := raw["Dimmer"].(float64); ok {
		l.Dimmer = new(uint(v))
		found = true
	}
	if v, ok := raw["HSBColor"].(string); ok {
		parts := strings.Split(v, ",")
		if len(parts) >= 2 {
			h, errH := strconv.Atoi(strings.TrimSpace(parts[0]))
			s, errS := strconv.Atoi(strings.TrimSpace(parts[1]))
			if errH == nil && errS == nil && h >= 0 && s >= 0 {
				l.Hue = new(uint(h))
				l.Saturation = new(uint(s))
				found = true
			}
		}
	}
	for k, v := range raw {
		idx, ok := PowerKeyIndex(k)
		if !ok {
			continue
		}
		if s, isStr := v.(string); isStr && (s == EventOn || s == EventOff) {
			l.PowerIndex = idx
			l.Power = new(s == EventOn)
		}
	}

	if !found && (l.Power == nil || l.PowerIndex == 1) {
		return nil, false
	}
	return l, true
}

// PowerKeyIndex maps "POWER" -> 1, "POWER3" -> 3.
func PowerKeyIndex(key string) (int, bool) {
	if key == "POWER" {
		return 1, true
	}
	if !strings.HasPrefix(key, "POWER") {
		return 0, false
	}
	n, err := strconv.Atoi(key[5:])
	if err != nil || n < 1 || n > 32 {
		return 0, false
	}
	return n, true
}
