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
		Case{nil, nil},
		Case{"foo", nil},
		Case{[]interface{}{"foo", "bar"}, nil},
		Case{[]interface{}{cb}, map[string]Path{"0": Path{0}}},
		Case{[]interface{}{"foo", "bar", cb}, map[string]Path{"0": Path{2}}},
		Case{[]interface{}{"foo", []interface{}{"bar", cb}}, map[string]Path{"0": Path{1, 1}}},
		Case{map[string]interface{}{"foo": 1, "bar": 2}, nil},
		Case{map[string]interface{}{"foo": 1, "bar": 2, "cb": cb}, map[string]Path{"0": Path{"cb"}}},
		Case{T{1, 2, cb, cb, nil}, map[string]Path{
			"0": Path{"c"},
			"1": Path{"f1"},
		}},
		Case{T{1, 2, cb, cb, &T{C: cb, d: cb}}, map[string]Path{
			"0": Path{"c"},
			"1": Path{"E", "c"},
			"2": Path{"E", "f1"},
			"3": Path{"E", "f3"},
			"4": Path{"f1"},
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
