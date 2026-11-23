package web

import (
	"reflect"
	"testing"
)

func TestParseSeriesIds(t *testing.T) {
	cases := []struct {
		in   []string
		want []uint
		ok   bool
	}{
		{[]string{""}, nil, true},
		{[]string{"all"}, nil, true},
		{[]string{"1,2"}, []uint{1, 2}, true},
		{[]string{"1", "3"}, []uint{1, 3}, true},
		{[]string{"1,x"}, nil, false},
		{[]string{"-1"}, nil, false},
	}
	for _, c := range cases {
		got, ok := parseSeriesIds(c.in)
		if ok != c.ok || (ok && !reflect.DeepEqual(got, c.want)) {
			t.Errorf("%q: %v %v, want %v %v", c.in, got, ok, c.want, c.ok)
		}
	}
}
