// Package logx writes one-line JSON logs to stderr.
//
// The CLI contract is strict: stdout carries data only, so every diagnostic
// line produced anywhere in the process goes through this logger to stderr.
package logx

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

// Level is the severity of a log record.
type Level string

// Log levels, ordered by severity.
const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

var rank = map[Level]int{
	LevelDebug: 0,
	LevelInfo:  1,
	LevelWarn:  2,
	LevelError: 3,
}

// Logger emits newline-delimited JSON records.
type Logger struct {
	mu  sync.Mutex
	w   io.Writer
	min Level
}

// New returns a logger writing to w at info level.
func New(w io.Writer) *Logger {
	if w == nil {
		w = io.Discard
	}
	return &Logger{w: w, min: LevelInfo}
}

// Stderr returns a logger writing to the process stderr.
func Stderr() *Logger { return New(os.Stderr) }

// Discard returns a logger that drops everything.
func Discard() *Logger { return New(io.Discard) }

// SetLevel changes the minimum emitted level.
func (l *Logger) SetLevel(lv Level) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.min = lv
}

func (l *Logger) emit(lv Level, msg string, kv []any) {
	if l == nil || l.w == nil {
		return
	}
	l.mu.Lock()
	min := l.min
	l.mu.Unlock()
	if rank[lv] < rank[min] {
		return
	}

	rec := make(map[string]any, 3+len(kv)/2)
	rec["ts"] = time.Now().Format(time.RFC3339Nano)
	rec["level"] = string(lv)
	rec["msg"] = msg
	for i := 0; i+1 < len(kv); i += 2 {
		key, ok := kv[i].(string)
		if !ok || key == "" {
			continue
		}
		switch key {
		case "ts", "level", "msg":
			continue
		}
		rec[key] = kv[i+1]
	}

	buf, err := json.Marshal(rec)
	if err != nil {
		return
	}
	buf = append(buf, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.w.Write(buf)
}

// Debug logs at debug level.
func (l *Logger) Debug(msg string, kv ...any) { l.emit(LevelDebug, msg, kv) }

// Info logs at info level.
func (l *Logger) Info(msg string, kv ...any) { l.emit(LevelInfo, msg, kv) }

// Warn logs at warn level.
func (l *Logger) Warn(msg string, kv ...any) { l.emit(LevelWarn, msg, kv) }

// Error logs at error level.
func (l *Logger) Error(msg string, kv ...any) { l.emit(LevelError, msg, kv) }
