// Package weibo is the only package that knows weibo's HTTP surface.
//
// When upstream changes an endpoint, request header, or response shape, the
// fix belongs here — nothing outside this package parses weibo payloads.
package weibo

import (
	"errors"
	"net/http"

	"github.com/yilong-musk/feedmesh/internal/apperr"
)

// The error envelope lives in internal/apperr; these aliases keep the
// package's own code unqualified.
const (
	CodeBadRequest          = apperr.CodeBadRequest
	CodeNetwork             = apperr.CodeNetwork
	CodeTimeout             = apperr.CodeTimeout
	CodeUpstreamBlocked     = apperr.CodeUpstreamBlocked
	CodeUpstreamUnavailable = apperr.CodeUpstreamUnavailable
	CodeUserNotFound        = apperr.CodeUserNotFound
	CodeLoginRequired       = apperr.CodeLoginRequired
	CodeBreakerOpen         = apperr.CodeBreakerOpen
	CodeUnauthorized        = apperr.CodeUnauthorized
	CodeRateLimited         = apperr.CodeRateLimited
	CodeInternal            = apperr.CodeInternal
)

// Error is the shared classified failure.
type Error = apperr.Error

// NewError builds an Error with a formatted message.
func NewError(code string, status int, format string, args ...any) *Error {
	return apperr.New(code, status, format, args...)
}

// CodeOf extracts the classification from any error, defaulting to INTERNAL.
func CodeOf(err error) string { return apperr.CodeOf(err) }

// StatusOf extracts the observed upstream status, or 0.
func StatusOf(err error) int { return apperr.StatusOf(err) }

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
