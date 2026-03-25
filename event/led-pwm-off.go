package event

import "strconv"

type LedPwmOff uint

func NewLedPwmOff(payload string) (LedPwmOff, error) {
	val, err := strconv.Atoi(payload)
	if err != nil {
		return 0, err
	}
	return LedPwmOff(val), nil
}
