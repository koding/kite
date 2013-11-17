package dnode

import (
	"encoding/json"
	"errors"
	"reflect"
)

// Partial is the type of "arguments" field in dnode.Message.
type Partial struct {
	Raw           []byte
	CallbackSpecs []CallbackSpec
}

// MarshalJSON returns the raw bytes of the Partial.
func (p *Partial) MarshalJSON() ([]byte, error) {
	return p.Raw, nil
}

// UnmarshalJSON puts the data into Partial.Raw.
func (p *Partial) UnmarshalJSON(data []byte) error {
	// p.Raw = make([]byte, len(data))
	// copy(p.Raw, data)
	// return nil
	if p == nil {
		return errors.New("json.Partial: UnmarshalJSON on nil pointer")
	}
	p.Raw = append(p.Raw[0:0], data...)
	return nil
}

// Unmarshal unmarshals the raw data (p.Raw) into v and prepares callbacks.
// v must be a struct that is the type of expected arguments.
func (p *Partial) Unmarshal(v interface{}) error {
	l.Printf("Unmarshal Partial")

	value := reflect.ValueOf(v)
	if value.Kind() != reflect.Ptr {
		panic("v must be a pointer")
	}

	err := json.Unmarshal(p.Raw, &v)
	if err != nil {
		l.Println(err)
		return err
	}

	for _, spec := range p.CallbackSpecs {
		l.Printf("spec: %#v", spec)
		err := spec.Apply(value)
		if err != nil {
			l.Println(err)
			return err
		}
	}

	return nil
}

// Array is a helper method that returns a []interface{}
// by unmarshalling the Partial.
func (p *Partial) Array() ([]interface{}, error) {
	var a []interface{}
	err := p.Unmarshal(&a)
	if err != nil {
		return nil, err
	}

	return a, nil
}

// Array is a helper method that returns a map[string]interface{}
// by unmarshalling the Partial.
func (p *Partial) Map() (map[string]interface{}, error) {
	var m map[string]interface{}
	err := p.Unmarshal(&m)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// String is a helper to unmarshal a string value.
func (p *Partial) String() (s string, err error) {
	err = p.Unmarshal(&s)
	return
}

// Float64 is a helper to unmarshal a float64 value.
func (p *Partial) Float64() (f float64, err error) {
	err = p.Unmarshal(&f)
	return
}
