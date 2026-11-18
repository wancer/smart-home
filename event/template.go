package event

// Template is the reply to the Tasmota `Template` command: {"NAME":..,"GPIO":[..],"FLAG":..,"BASE":..}.
// Only the GPIO component codes are kept.
type Template struct {
	Gpio []int
}

func NewTemplate(raw []any) Template {
	codes := make([]int, 0, len(raw))
	for _, v := range raw {
		if f, ok := v.(float64); ok {
			codes = append(codes, int(f))
		}
	}
	return Template{Gpio: codes}
}
