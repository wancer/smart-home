package event

import (
	"fmt"
	"slices"
)

const (
	EventOn  = "ON"
	EventOff = "OFF"
)

var PossibleOnOff = []string{EventOn, EventOff}

type LedPower bool

func NewLedPower(payload string) (LedPower, error) {
	if !slices.Contains(PossibleOnOff, payload) {
		return LedPower(false), fmt.Errorf("unkown value %s", payload)
	}

	return LedPower(payload == "ON"), nil
}
