package automation

import (
	"smart-home/model"
	"testing"
)

func TestSeriesName(t *testing.T) {
	label := "Kitchen"
	all := []model.SensorSeries{
		{Id: 1, Source: "BH1750", Field: "Illuminance", Kind: model.KindIlluminance},
		{Id: 2, Source: "TCS34725", Field: "Illuminance", Kind: model.KindIlluminance},
		{Id: 3, Source: "SHT4X", Field: "Temperature", Kind: model.KindTemperature, Label: &label},
		{Id: 4, Source: "TCS34725", Field: "Red", Kind: model.KindRaw},
		{Id: 5, Source: "MQ", Field: "Ratio", Kind: model.KindGasRatio},
	}
	want := []string{"illuminance (BH1750)", "illuminance (TCS34725)", "Kitchen", "TCS34725.Red", "gas ratio"}
	for i, s := range all {
		if got := SeriesName(s, all); got != want[i] {
			t.Errorf("%d: %q, want %q", s.Id, got, want[i])
		}
	}
}

func TestFormatSeriesValue(t *testing.T) {
	cases := map[string]string{
		FormatSeriesValue(model.KindTemperature, 21.46): "21.5°C",
		FormatSeriesValue(model.KindCo2, 830.4):         "830 ppm",
		FormatSeriesValue(model.KindGasRatio, 1.234):    "1.23",
		FormatSeriesValue("unknown", 2):                 "2.00",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("%q, want %q", got, want)
		}
	}
}
