package contracts

import (
	"encoding/json"
	"errors"
	"net"
	"time"
)

type PollCategory string

const (
	PollTimeout   PollCategory = "POLL_TRANSPORT_TIMEOUT"
	PollTransport PollCategory = "POLL_TRANSPORT_OTHER"
	PollHTTP      PollCategory = "POLL_HTTP_STATUS"
	PollDecode    PollCategory = "POLL_DECODE"
	PollTemporary PollCategory = "POLL_PROVIDER_TEMPORARY"
	PollUnknown   PollCategory = "POLL_PERMANENT_OR_UNKNOWN"
)

func (category PollCategory) Valid() bool {
	switch category {
	case PollTimeout, PollTransport, PollHTTP, PollDecode, PollTemporary, PollUnknown:
		return true
	}
	return false
}

// PollError preserves Is/As, while its rendering and metadata never incorporate
// arbitrary upstream error strings, URLs, response bodies or task references.
type PollError struct {
	Category   PollCategory
	Retryable  bool
	HTTPStatus int
	Elapsed    time.Duration
	Cause      error
}

func (err *PollError) Error() string { return string(err.Category) }

// A boolean alone cannot make an unknown/permanent/decode error retryable.
func (err *PollError) CanRetry() bool {
	if !err.Retryable {
		return false
	}
	switch err.Category {
	case PollTimeout, PollTemporary:
		return true
	case PollHTTP:
		return err.HTTPStatus == 429 || err.HTTPStatus >= 500 && err.HTTPStatus <= 599
	default:
		return false
	}
}
func (err *PollError) Unwrap() error                    { return err.Cause }
func (err *PollError) DiagnosticCategory() string       { return string(err.Category) }
func (err *PollError) DiagnosticElapsed() time.Duration { return err.Elapsed }
func (err *PollError) DiagnosticHTTPStatus() int        { return err.HTTPStatus }
func (err *PollError) DiagnosticCauseType() string {
	var syntax *json.SyntaxError
	var typed *json.UnmarshalTypeError
	var dns *net.DNSError
	var operation *net.OpError
	switch {
	case errors.As(err.Cause, &syntax):
		return "*json.SyntaxError"
	case errors.As(err.Cause, &typed):
		return "*json.UnmarshalTypeError"
	case errors.As(err.Cause, &dns):
		return "*net.DNSError"
	case errors.As(err.Cause, &operation):
		return "*net.OpError"
	}
	var network net.Error
	if errors.As(err.Cause, &network) {
		return "NET_ERROR"
	}
	return "UNKNOWN"
}
