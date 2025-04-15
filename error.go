package apic

import (
	"errors"
	"fmt"
)

var (
	MaxAttemptsError = errors.New("max connection attempts")
)

type ResponseError struct {
	Code int
	Body []byte
}

func (e ResponseError) Error() string {
	return fmt.Sprintf("http code %d: %s", e.Code, string(e.Body))
}

type DecodeError struct {
	Body []byte
	Err  error
}

func (e DecodeError) Error() string {
	return fmt.Sprintf("decode: %s: %s", e.Err, string(e.Body))
}

func GetResponseErrorCode(err error) int {
	e, ok := err.(ResponseError)
	if !ok {
		return 0
	}
	return e.Code
}
