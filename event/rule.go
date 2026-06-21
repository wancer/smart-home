package event

type Rule struct {
	N     int
	State int
	Once  int
	Rules string
	Free  int
}

func NewRule(n int, data map[string]any) Rule {
	r := Rule{N: n}
	if v, ok := data["State"]; ok {
		if fv, ok := v.(float64); ok {
			r.State = int(fv)
		}
	}
	if v, ok := data["Once"]; ok {
		if fv, ok := v.(float64); ok {
			r.Once = int(fv)
		}
	}
	if v, ok := data["Rules"]; ok {
		if sv, ok := v.(string); ok {
			r.Rules = sv
		}
	}
	if v, ok := data["Free"]; ok {
		if fv, ok := v.(float64); ok {
			r.Free = int(fv)
		}
	}
	return r
}
