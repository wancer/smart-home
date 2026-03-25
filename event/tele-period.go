package event

import "strconv"

type TelePeriod uint

func NewTelePeriod(payload string) (TelePeriod, error) {
	val, err := strconv.Atoi(payload)
	if err != nil {
		return 0, err
	}
	return TelePeriod(val), nil
}
