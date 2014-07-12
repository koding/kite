package dnode

import (
	"encoding/json"
	"errors"
	"fmt"
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
	if p == nil {
		return errors.New("json.Partial: UnmarshalJSON on nil pointer")
	}

	p.Raw = make([]byte, len(data))
	copy(p.Raw, data)
	return nil
}

// Unmarshal unmarshals the raw data (p.Raw) into v and prepares callbacks.
// v must be a struct that is the type of expected arguments.
func (p *Partial) Unmarshal(v interface{}) error {
	if p == nil {
		return fmt.Errorf("Cannot unmarshal nil argument")
	}

	if err := json.Unmarshal(p.Raw, &v); err != nil {
		return fmt.Errorf("%s. Data: %s", err.Error(), string(p.Raw))
	}

	value := reflect.ValueOf(v)

	for _, spec := range p.CallbackSpecs {
		if err := setCallback(value, spec.Path, spec.Function.Caller.(functionReceived)); err != nil {
			return err
		}
	}

	return nil
}

func (p *Partial) MustUnmarshal(v interface{}) {
	err := p.Unmarshal(v)
	checkError(err)
}

//-------------------------------------------
// Helper methods for unmarshaling JSON types
//-------------------------------------------

// Slice is a helper method to unmarshal a JSON Array.
func (p *Partial) Slice() (a []*Partial, err error) {
	err = p.Unmarshal(&a)
	return
}

// SliceOfLength is a helper method to unmarshal a JSON Array with specified length.
func (p *Partial) SliceOfLength(length int) (a []*Partial, err error) {
	err = p.Unmarshal(&a)
	if err != nil {
		return
	}

	if len(a) != length {
		err = errors.New("Invalid array length")
	}

	return
}

// Map is a helper method to unmarshal to a JSON Object.
func (p *Partial) Map() (m map[string]*Partial, err error) {
	err = p.Unmarshal(&m)
	return
}

// String is a helper to unmarshal a JSON String.
func (p *Partial) String() (s string, err error) {
	err = p.Unmarshal(&s)
	return
}

// Float64 is a helper to unmarshal a JSON Number.
func (p *Partial) Float64() (f float64, err error) {
	err = p.Unmarshal(&f)
	return
}

// Bool is a helper to unmarshal a JSON Boolean.
func (p *Partial) Bool() (b bool, err error) {
	err = p.Unmarshal(&b)
	return
}

// Function is a helper to unmarshal a callback function.
func (p *Partial) Function() (f Function, err error) {
	err = p.Unmarshal(&f)
	return
}

//----------------------------------------------------------------
// Helper methods for unmarshaling JSON types that panic on errors
//----------------------------------------------------------------

func checkError(err error) {
	if err != nil {
		panic(&ArgumentError{err.Error()})
	}
}

func (p *Partial) MustSlice() []*Partial {
	a, err := p.Slice()
	checkError(err)
	return a
}

func (p *Partial) MustSliceOfLength(length int) []*Partial {
	a, err := p.SliceOfLength(length)
	checkError(err)
	return a
}

func (p *Partial) One() *Partial {
	return p.MustSliceOfLength(1)[0]
}

func (p *Partial) MustMap() map[string]*Partial {
	m, err := p.Map()
	checkError(err)
	return m
}

func (p *Partial) MustString() string {
	s, err := p.String()
	checkError(err)
	return s
}

func (p *Partial) MustFloat64() float64 {
	f, err := p.Float64()
	checkError(err)
	return f
}

func (p *Partial) MustBool() bool {
	b, err := p.Bool()
	checkError(err)
	return b
}

func (p *Partial) MustFunction() Function {
	f, err := p.Function()
	checkError(err)
	return f
}
