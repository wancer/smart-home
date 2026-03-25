package event

import (
	"fmt"
	"slices"
)

type Power bool

func NewPower(payload string) (Power, error) {
	if !slices.Contains(PossibleOnOff, payload) {
		return Power(false), fmt.Errorf("unkown value %s", payload)
	}

	return Power(payload == "ON"), nil
}
