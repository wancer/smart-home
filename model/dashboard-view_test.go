package model

import (
	"reflect"
	"testing"
)

func TestDashboardViewFromLegacy(t *testing.T) {
	preferred := map[string][]uint{KindTemperature: {7, 3}, KindCo2: {4}, KindPower: {9}}

	view, err := LegacyDashboardView{Chart: "temperature", Values: []string{"co2", "temperature", "pressure"}}.FromLegacy(preferred)
	if err != nil {
		t.Fatal(err)
	}
	want := DashboardView{Chart: new(uint(7)), Values: []uint{4, 7}}
	if !reflect.DeepEqual(view, want) {
		t.Errorf("FromLegacy: %+v, want %+v", view, want)
	}
	if _, err := (LegacyDashboardView{Values: []string{"bogus"}}).FromLegacy(preferred); err == nil {
		t.Error("unknown metric accepted")
	}
	if v, _ := (LegacyDashboardView{Chart: "pressure"}).FromLegacy(preferred); !v.IsAuto() {
		t.Errorf("chart without a series: %+v, want auto", v)
	}
	if v, _ := (LegacyDashboardView{Chart: "none"}).FromLegacy(preferred); !v.NoChart {
		t.Errorf("none: %+v", v)
	}
}

func TestValidateDashboardView(t *testing.T) {
	owned := func(id uint) bool { return id < 10 }
	cases := []struct {
		view DashboardView
		ok   bool
	}{
		{DashboardView{}, true},
		{DashboardView{Chart: new(uint(3)), Values: []uint{1, 2}}, true},
		{DashboardView{Chart: new(uint(11))}, false},
		{DashboardView{Values: []uint{1, 11}}, false},
		{DashboardView{Values: []uint{1, 1}}, false},
		{DashboardView{Chart: new(uint(1)), NoChart: true}, false},
	}
	for _, c := range cases {
		if err := ValidateDashboardView(c.view, owned); (err == nil) != c.ok {
			t.Errorf("%+v: err %v, want ok=%v", c.view, err, c.ok)
		}
	}
}
