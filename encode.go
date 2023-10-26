package apic

import (
	"encoding/json"
)

type Encoder func(any) ([]byte, error)

type Decoder func([]byte, any) error

var (
	defaultEncoder = json.Marshal
	defaultDecoder = json.Unmarshal
)
