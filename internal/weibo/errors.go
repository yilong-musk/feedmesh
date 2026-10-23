// Package weibo is the only package that knows weibo's HTTP surface.
//
// When upstream changes an endpoint, request header, or response shape, the
// fix belongs here — nothing outside this package parses weibo payloads.
package weibo

import (
	"errors"
	"fmt"
	"net/http"
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

// NewError builds an Error with a formatted message.
func NewError(code string, status int, format string, args ...any) *Error {
	return &Error{Code: code, Status: status, Message: fmt.Sprintf(format, args...)}
}

// CodeOf extracts the classification from any error, defaulting to INTERNAL.
func CodeOf(err error) string {
	var we *Error
	if errors.As(err, &we) {
		return we.Code
	}
	return CodeInternal
}

// StatusOf extracts the observed upstream status, or 0.
func StatusOf(err error) int {
	var we *Error
	if errors.As(err, &we) {
		return we.Status
	}
	return 0
}

// IsBlocked reports whether err came from a 418/403 upstream block.
func IsBlocked(err error) bool {
	var we *Error
	if !errors.As(err, &we) {
		return false
	}
	return we.Code == CodeUpstreamBlocked ||
		we.Status == http.StatusTeapot ||
		we.Status == http.StatusForbidden
}
