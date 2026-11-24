package model

import "testing"

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
