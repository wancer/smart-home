package event

// Template is the reply to the Tasmota `Template` command: {"NAME":..,"GPIO":[..],"FLAG":..,"BASE":..}.
type Template struct {
	Name string
	Gpio []int
	Flag int
	Base int
}

func NewTemplate(raw map[string]any) Template {
	gpio, _ := raw["GPIO"].([]any)
	codes := make([]int, 0, len(gpio))
	for _, v := range gpio {
		if f, ok := v.(float64); ok {
			codes = append(codes, int(f))
		}
	}
	t := Template{Gpio: codes}
	t.Name, _ = raw["NAME"].(string)
	if f, ok := raw["FLAG"].(float64); ok {
		t.Flag = int(f)
	}
	if f, ok := raw["BASE"].(float64); ok {
		t.Base = int(f)
	}
	return t
}
