// Package httpapi exposes the feed over HTTP.
package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/yilong-musk/weibo-rss-pro/internal/weibo"
)

// ErrorCodeHeader carries the machine-readable error code.
const ErrorCodeHeader = "X-Weibo-RSS-Code"

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

// statusFor maps an internal error code onto an HTTP status.
func statusFor(code string) int {
	switch code {
	case weibo.CodeBadRequest:
		return http.StatusBadRequest
	case weibo.CodeUserNotFound, weibo.CodeLoginRequired:
		return http.StatusNotFound
	case weibo.CodeUpstreamBlocked:
		return http.StatusBadGateway
	case weibo.CodeBreakerOpen:
		return http.StatusServiceUnavailable
	case weibo.CodeNetwork, weibo.CodeTimeout, weibo.CodeUpstreamUnavailable:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

func (s *Server) writeError(w http.ResponseWriter, err error) {
	code := weibo.CodeOf(err)
	s.writeErrorStatus(w, statusFor(code), err)
}

// writeErrorStatus emits the error envelope with an explicit HTTP status, for
// cases where the status carries information the code alone does not (405).
func (s *Server) writeErrorStatus(w http.ResponseWriter, status int, err error) {
	code := weibo.CodeOf(err)
	w.Header().Set(ErrorCodeHeader, code)
	writeJSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: err.Error()}})
}
