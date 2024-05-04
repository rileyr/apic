package apic

import (
	"fmt"
	"io"
	"net/http"
)

func badStatusError(rsp *http.Response) error {
	bts, err := io.ReadAll(rsp.Body)
	if err != nil {
		return err
	}
	return statusError{
		body: bts,
		code: rsp.StatusCode,
	}
}

type statusError struct {
	body []byte
	code int
}

func (se statusError) Error() string {
	return fmt.Sprintf("api returned bad status: %d [%d]", se.code, len(se.body))
}

func GetErrorCode(err error) int {
	se, ok := err.(statusError)
	if !ok {
		return 0
	}
	return se.code
}
