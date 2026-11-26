// Package apperr is the error envelope shared by every provider, the CLI and
// the HTTP API: a classified failure with a stable code.
//
// It used to live in internal/weibo, which made every other provider depend on
// the Weibo package for its error type.
package apperr

import (
	"errors"
	"fmt"
)

// Error codes surfaced to the CLI, the JSON envelope, and the HTTP API.
const (
	CodeBadRequest          = "BAD_REQUEST"
	CodeNetwork             = "NETWORK"
	CodeTimeout             = "TIMEOUT"
	CodeUpstreamBlocked     = "UPSTREAM_BLOCKED"
	CodeUpstreamUnavailable = "UPSTREAM_UNAVAILABLE"
	CodeUserNotFound        = "USER_NOT_FOUND"
	CodeLoginRequired       = "LOGIN_REQUIRED"
	CodeBreakerOpen         = "BREAKER_OPEN"
	CodeUnauthorized        = "UNAUTHORIZED"
	CodeRateLimited         = "RATE_LIMITED"
	CodeInternal            = "INTERNAL"
)

// Error is a classified upstream failure.
type Error struct {
	Code    string
	Message string
	Status  int // upstream HTTP status when one was observed
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Code
}

// Unwrap exposes the wrapped cause.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// New builds an Error with a formatted message.
func New(code string, status int, format string, args ...any) *Error {
	return &Error{Code: code, Status: status, Message: fmt.Sprintf(format, args...)}
}

// CodeOf extracts the classification from any error, defaulting to INTERNAL.
func CodeOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return CodeInternal
}

// StatusOf extracts the observed upstream status, or 0.
func StatusOf(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.Status
	}
	return 0
}
