package event

import (
	"fmt"
	"slices"
)

type LedPwmMode bool

func NewLedPwmMode(payload string) (LedPwmMode, error) {
	if !slices.Contains(PossibleOnOff, payload) {
		return LedPwmMode(false), fmt.Errorf("unkown value %s", payload)
	}

	return LedPwmMode(payload == "ON"), nil
}
