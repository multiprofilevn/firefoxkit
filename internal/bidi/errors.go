package bidi

import (
	"errors"
	"fmt"
)

// ErrClientClosed is returned by Send when the Client is Closed before a
// response arrives. Pending commands are drained with this error.
var ErrClientClosed = errors.New("bidi: client closed")

// ErrCommandTimeout is returned when a command's context expires before
// the matching response arrives. The reader goroutine drops the late
// response on arrival.
var ErrCommandTimeout = errors.New("bidi: command timeout")

// ErrMalformedFrame is logged (not returned) when the reader receives a
// frame that is neither a valid response nor a valid event. Exposed for
// tests that want to assert on the underlying cause.
var ErrMalformedFrame = errors.New("bidi: malformed frame")

// BiDiError is the typed representation of a BiDi error response. Callers
// can errors.As this out of the error returned by Send.
type BiDiError struct {
	Code       string
	Message    string
	Stacktrace string
}

func (e *BiDiError) Error() string {
	if e.Stacktrace != "" {
		return fmt.Sprintf("bidi: %s: %s\n%s", e.Code, e.Message, e.Stacktrace)
	}
	return fmt.Sprintf("bidi: %s: %s", e.Code, e.Message)
}
