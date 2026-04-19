package event

type TimeDst struct {
	Day        uint
	Hemisphere uint
	Hour       uint
	Month      uint
	Offset     uint
	Week       uint
}

func NewTimeDst(in map[string]any) TimeDst {
	return TimeDst{
		Day:        uint(in["Day"].(float64)),
		Hemisphere: uint(in["Hemisphere"].(float64)),
		Hour:       uint(in["Hour"].(float64)),
		Month:      uint(in["Month"].(float64)),
		Offset:     uint(in["Offset"].(float64)),
		Week:       uint(in["Week"].(float64)),
	}
}
