package httpapi

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

// ErrorLog adapts the standard library's http.Server error logger to the
// newline-delimited JSON logger used everywhere else.
func ErrorLog(l *logx.Logger) *log.Logger {
	if l == nil {
		l = logx.Discard()
	}
	return log.New(errorLogWriter{log: l}, "", 0)
}

type errorLogWriter struct{ log *logx.Logger }

func (w errorLogWriter) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if msg != "" {
		w.log.Error(msg, "component", "http")
	}
	return len(p), nil
}

func (s *Server) withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tw := &trackingWriter{ResponseWriter: w}
		defer func() {
			if rec := recover(); rec != nil {
				s.cfg.Log.Error("panic serving request",
					"method", r.Method, "path", r.URL.Path, "panic", fmt.Sprint(rec))
				if !tw.wroteHeader {
					s.writeErrorStatus(tw, http.StatusInternalServerError,
						weibo.NewError(weibo.CodeInternal, 0, "internal server error"))
				}
			}
		}()
		next.ServeHTTP(tw, r)
	})
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	want := []byte("Bearer " + s.cfg.Token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(got, want) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="feedmesh"`)
			w.Header().Set("Cache-Control", "no-store")
			s.writeErrorStatus(w, http.StatusUnauthorized,
				weibo.NewError(weibo.CodeUnauthorized, 0, "missing or invalid bearer token"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withRateLimit(next http.Handler) http.Handler {
	limiter := newRateLimiter(s.cfg.RateLimit)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "60")
			w.Header().Set("Cache-Control", "no-store")
			s.writeErrorStatus(w, http.StatusTooManyRequests,
				weibo.NewError(weibo.CodeRateLimited, 0, "too many requests; try again later"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withTimeout(next http.Handler) http.Handler {
	timeout := s.cfg.RequestTimeout
	if timeout <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type trackingWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *trackingWriter) WriteHeader(code int) {
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *trackingWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(p)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

// rateLimiter enforces a rolling one-minute window per key. A fixed window
// would allow a burst of up to 2x the limit across its boundary.
type rateLimiter struct {
	mu        sync.Mutex
	limit     int
	now       func() time.Time
	lastSweep time.Time
	windows   map[string][]time.Time
}

func newRateLimiter(limit int) *rateLimiter {
	return &rateLimiter{limit: limit, now: time.Now}
}

func (l *rateLimiter) allow(key string) bool {
	if l.limit <= 0 {
		return true
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.windows == nil {
		l.windows = make(map[string][]time.Time)
		l.lastSweep = now
	}
	// Drop idle keys so a stream of unique clients cannot grow the map forever.
	if now.Sub(l.lastSweep) >= time.Minute {
		cutoff := now.Add(-time.Minute)
		for k, stamps := range l.windows {
			if len(stamps) == 0 || !stamps[len(stamps)-1].After(cutoff) {
				delete(l.windows, k)
			}
		}
		l.lastSweep = now
	}

	cutoff := now.Add(-time.Minute)
	stamps := l.windows[key]
	first := 0
	for first < len(stamps) && !stamps[first].After(cutoff) {
		first++
	}
	stamps = stamps[first:]
	if len(stamps) >= l.limit {
		l.windows[key] = stamps
		return false
	}
	l.windows[key] = append(stamps, now)
	return true
}
