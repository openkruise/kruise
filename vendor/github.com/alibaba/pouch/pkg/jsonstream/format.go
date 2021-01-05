package jsonstream

import (
	"encoding/json"
)

// Formater define the data's format via stream transfer.
type Formater interface {
	// BeginWrite write some data at the stream beginning.
	BeginWrite() ([]byte, error)

	// EndWrite write some data at the stream ending.
	EndWrite() ([]byte, error)

	// Write write a object be serialized.
	Write(o interface{}) ([]byte, error)
}

// defaultFormat define the pouch's pull progress.
type defaultFormat struct{}

func newDefaultFormat() Formater {
	return &defaultFormat{}
}

func (f *defaultFormat) BeginWrite() ([]byte, error) {
	return nil, nil
}

func (f *defaultFormat) EndWrite() ([]byte, error) {
	return nil, nil
}

func (f *defaultFormat) Write(o interface{}) ([]byte, error) {
	return json.Marshal(o)
}
