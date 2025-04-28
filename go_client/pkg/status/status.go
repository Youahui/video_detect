package status

import (
	"errors"
	"net/http"
)

type Error struct {
	StatusCode int
	Message    string
}

func (e *Error) Error() string {
	switch {
	case e == nil:
		return "<nil>"
	case e.Message == "":
		return http.StatusText(e.StatusCode)
	default:
		return e.Message
	}
}

func WrapperE(code int, message string) error {
	return &Error{
		StatusCode: code,
		Message:    message,
	}
}

func Wrapper(code int, err error) error {
	return &Error{
		StatusCode: code,
		Message:    err.Error(),
	}
}

func As(err error) (*Error, bool) {
	var statusErr *Error
	return statusErr, errors.As(err, &statusErr)
}
