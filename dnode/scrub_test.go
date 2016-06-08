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
		{[]interface{}{cb, "foo", cb}, map[string]Path{"0": {0}, "1": {2}}},
		{[]interface{}{"foo", "bar", cb}, map[string]Path{"0": {2}}},
		{[]interface{}{"foo", []interface{}{"bar", cb}}, map[string]Path{"0": {1, 1}}},
		{[...]interface{}{"foo", cb, cb}, map[string]Path{"0": {1}, "1": {2}}},
		{map[string]interface{}{"foo": 1, "bar": 2}, nil},
		{map[string]interface{}{"foo": 1, "bar": 2, "cb": cb}, map[string]Path{"0": {"cb"}}},
		{T{privT{0, cb, cb}, 1, 2, cb, cb, nil}, map[string]Path{
			"0": {"embedB"},
			"1": {"c"},
			"2": {"f1"},
		}},
		{T{A: 1, b: 2, C: cb, d: cb, E: &T{C: cb, d: cb}}, map[string]Path{
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

type privT struct {
	A int
	B Function `json:"embedB"`
	F Function `dnode:"-"`
}

type T struct {
	privT
	A int
	b int
	C Function `json:"c,omitempty"`
	d Function
	E *T
}

// Combination of exported/unexported value/pointer receiver methods.
func (t T) F1(p *Partial)  {}
func (t T) f2(p *Partial)  {}
func (t *T) F3(p *Partial) {}
func (t *T) f4(p *Partial) {}
