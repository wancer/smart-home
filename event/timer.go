package event

type Timer struct {
	N      int
	Enable int
	Mode   int
	Time   string
	Window int
	Days   string
	Repeat int
	Output int
	Action int
}

func NewTimer(n int, data map[string]any) Timer {
	t := Timer{N: n, Output: 1, Time: "00:00", Days: "0000000"}
	if v, ok := data["Enable"]; ok {
		if fv, ok := v.(float64); ok {
			t.Enable = int(fv)
		}
	}
	if v, ok := data["Mode"]; ok {
		if fv, ok := v.(float64); ok {
			t.Mode = int(fv)
		}
	}
	if v, ok := data["Time"]; ok {
		if sv, ok := v.(string); ok {
			t.Time = sv
		}
	}
	if v, ok := data["Window"]; ok {
		if fv, ok := v.(float64); ok {
			t.Window = int(fv)
		}
	}
	if v, ok := data["Days"]; ok {
		if sv, ok := v.(string); ok {
			t.Days = sv
		}
	}
	if v, ok := data["Repeat"]; ok {
		if fv, ok := v.(float64); ok {
			t.Repeat = int(fv)
		}
	}
	if v, ok := data["Output"]; ok {
		if fv, ok := v.(float64); ok {
			t.Output = int(fv)
		}
	}
	if v, ok := data["Action"]; ok {
		if fv, ok := v.(float64); ok {
			t.Action = int(fv)
		}
	}
	return t
}
