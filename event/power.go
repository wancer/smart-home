package event

import (
	"fmt"
	"slices"
)

// Power is the state of one or more POWER<n> outputs (relays, or the light after them), keyed
// by index (1 = POWER / POWER1). It comes from stat/POWER, stat/RESULT ({"POWER":"ON"},
// {"POWER2":"OFF"}, a multi-relay device answers POWER with {"POWER1":…}) and, with every
// output at once, from tele/STATE and the STATUS11 reply.
type Power struct {
	States map[int]bool
	// Tele: from tele/STATE / STATUS11, which never bring a device online (applied only while online).
	Tele bool
}

// NewPower parses the stat/POWER payload ("ON" / "OFF") of output index.
func NewPower(payload string, index int) (Power, error) {
	if !slices.Contains(PossibleOnOff, payload) {
		return Power{}, fmt.Errorf("unkown value %s", payload)
	}

	return Power{States: map[int]bool{index: payload == EventOn}}, nil
}

// NewPowerFromMap picks the POWER<n> keys of a JSON object; false if there are none.
func NewPowerFromMap(raw map[string]any) (*Power, bool) {
	p := &Power{States: map[int]bool{}}
	for k, v := range raw {
		idx, ok := PowerKeyIndex(k)
		if !ok {
			continue
		}
		if s, isStr := v.(string); isStr && (s == EventOn || s == EventOff) {
			p.States[idx] = s == EventOn
		}
	}
	return p, len(p.States) > 0
}

// OnlyPowerKeys reports whether every key of the object is a POWER<n> key.
func OnlyPowerKeys(raw map[string]any) bool {
	for k := range raw {
		if _, ok := PowerKeyIndex(k); !ok {
			return false
		}
	}
	return len(raw) > 0
}
