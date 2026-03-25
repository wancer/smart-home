package event

import "strconv"

type LedPwmOn uint

func NewLedPwmOn(payload string) (LedPwmOn, error) {
	val, err := strconv.Atoi(payload)
	if err != nil {
		return 0, err
	}
	return LedPwmOn(val), nil
}
