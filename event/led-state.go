package event

import "strconv"

type LedState uint

func NewLedState(payload string) (LedState, error) {
	val, err := strconv.Atoi(payload)
	if err != nil {
		return 0, err
	}
	state := LedState(val)
	return state, nil
}
