package dnode

import (
	"reflect"
	"testing"
)

func TestScrub(t *testing.T) {
	type Case struct {
		obj       interface{}
		callbacks map[string]Path
	}

	cb := Callback(func(*Partial) {})

	cases := []Case{
		{nil, nil},
		{"foo", nil},
		{[]interface{}{"foo", "bar"}, nil},
		{[]interface{}{cb}, map[string]Path{"0": {0}}},
		{[]interface{}{"foo", "bar", cb}, map[string]Path{"0": {2}}},
		{[]interface{}{"foo", []interface{}{"bar", cb}}, map[string]Path{"0": {1, 1}}},
		{map[string]interface{}{"foo": 1, "bar": 2}, nil},
		{map[string]interface{}{"foo": 1, "bar": 2, "cb": cb}, map[string]Path{"0": {"cb"}}},
		{T{1, 2, cb, cb, nil}, map[string]Path{
			"0": {"c"},
			"1": {"f1"},
		}},
		{T{1, 2, cb, cb, &T{C: cb, d: cb}}, map[string]Path{
			"0": {"c"},
			"1": {"E", "c"},
			"2": {"E", "f1"},
			"3": {"E", "f3"},
			"4": {"f1"},
		}},
	}

	for i, c := range cases {
		scrubber := NewScrubber()
		callbacks := scrubber.Scrub(c.obj)
		if len(callbacks) == 0 && len(c.callbacks) == 0 {
			continue
		}
		if !reflect.DeepEqual(callbacks, c.callbacks) {
			t.Errorf("test case %d, expected: %+v", i, c.callbacks)
			t.Errorf("got: %+v", callbacks)
		}
	}
}

type T struct {
	A int
	b int
	C Function `json:"c"`
	d Function
	E *T
}

// Combination of exported/unexported value/pointer receiver methods.
func (t T) F1(p *Partial)  {}
func (t T) f2(p *Partial)  {}
func (t *T) F3(p *Partial) {}
func (t *T) f4(p *Partial) {}
