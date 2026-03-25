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

func NewLedPower(payload string) (LedPwmMode, error) {
	if !slices.Contains(PossibleOnOff, payload) {
		return LedPwmMode(false), fmt.Errorf("unkown value %s", payload)
	}

	return LedPwmMode(payload == "ON"), nil
}
