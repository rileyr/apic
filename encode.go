package apic

import (
	"encoding/json"
)

type Encoder func(any) ([]byte, error)

type Decoder func([]byte, any) error

func defaultDecoder() Decoder {
	return func(bts []byte, dest any) error {
		return json.Unmarshal(bts, dest)
	}
}

func defaultEncoder() Encoder {
	return func(obj any) ([]byte, error) {
		return json.Marshal(obj)
	}
}
