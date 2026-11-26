// Package douyin reads a Douyin creator's public timeline.
//
// Douyin signs every web API call with a browser-generated token (a_bogus) and
// a visitor identity, and the signed token cannot be reproduced outside a real
// page. Rather than reimplementing that stack, this package drives a headless
// Chromium over the DevTools pipe protocol and issues the request from inside
// the page: the page's own SDK signs it, and the caller only parses the JSON.
//
// The protocol is implemented with the standard library only. Chromium exposes
// it on file descriptors 3 (commands in) and 4 (responses out) when started
// with --remote-debugging-pipe; messages are JSON documents separated by a NUL
// byte. That keeps the feedmesh binary free of third-party modules.
package douyin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// browser is one Chromium process speaking CDP over pipes.
type browser struct {
	cmd    *exec.Cmd
	in     *os.File
	out    *bufio.Reader
	mu     sync.Mutex
	seq    int
	closed bool
	// dead marks a browser whose protocol stream can no longer be trusted (a
	// read timed out mid-frame, or the pipe broke). It is atomic so the
	// client can check it without waiting for a call that holds mu.
	dead atomic.Bool

	// session is the flattened target session used for page commands.
	session string
	// warm records that a page fetch already succeeded, so the next call can
	// skip the visitor-cookie warm-up.
	warm atomic.Bool
	// lastUsed is touched on every successful request; the idle reaper closes
	// the process when it stays unused for a while.
	lastUsed atomic.Int64
	// ioTimeout bounds one protocol exchange: a broken renderer otherwise
	// blocks the pipe read forever and pins the HTTP request with it.
	ioTimeout time.Duration
	// stderr keeps the tail of Chromium's diagnostics for error messages.
	stderr *tailBuffer
}

type cdpMessage struct {
	ID        int             `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    any             `json:"params,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// defaultChromeCandidates are tried in order when no explicit path is set.
var defaultChromeCandidates = []string{
	"chromium",
	"chromium-browser",
	"google-chrome",
	"google-chrome-stable",
	"chrome",
}

// findChrome resolves the Chromium binary: explicit path, config path, then
// PATH lookup, then the Hermes bundled build used by this machine.
func findChrome(explicit string) (string, error) {
	candidates := make([]string, 0, len(defaultChromeCandidates)+2)
	if strings.TrimSpace(explicit) != "" {
		candidates = append(candidates, strings.TrimSpace(explicit))
	}
	if env := strings.TrimSpace(os.Getenv("FEEDMESH_CHROME")); env != "" {
		candidates = append(candidates, env)
	}
	candidates = append(candidates, defaultChromeCandidates...)
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p, nil
		}
	}
	// Hermes ships Chromium under a versioned directory; the path moves on
	// every upgrade, so resolve the newest match instead of pinning one.
	if home, err := os.UserHomeDir(); err == nil {
		if matches, err := filepath.Glob(home + "/.hermes/tools/chromium-*/chrome-linux64/chrome"); err == nil && len(matches) > 0 {
			return matches[len(matches)-1], nil
		}
	}
	return "", errors.New("chromium not found: install chromium or set FEEDMESH_CHROME")
}

func startBrowser(chromePath, profileDir string, logf func(string, ...any)) (*browser, error) {
	chrome, err := findChrome(chromePath)
	if err != nil {
		return nil, err
	}
	if profileDir != "" {
		if err := os.MkdirAll(profileDir, 0o700); err != nil {
			return nil, fmt.Errorf("create chromium profile: %w", err)
		}
	}

	toChromeR, toChromeW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	fromChromeR, fromChromeW, err := os.Pipe()
	if err != nil {
		toChromeR.Close()
		toChromeW.Close()
		return nil, err
	}

	args := []string{
		"--headless=new",
		"--no-sandbox",
		"--disable-dev-shm-usage",
		"--disable-gpu",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"--disable-sync",
		"--mute-audio",
		"--window-size=1280,900",
		"--lang=zh-CN",
		"--disable-blink-features=AutomationControlled",
		"--user-agent=" + userAgent,
		"--remote-debugging-pipe",
	}
	if profileDir != "" {
		args = append(args, "--user-data-dir="+profileDir)
	}
	args = append(args, "about:blank")

	cmd := exec.Command(chrome, args...)
	cmd.SysProcAttr = sysProcAttr()
	cmd.ExtraFiles = []*os.File{toChromeR, fromChromeW}
	tail := newTailBuffer(8 << 10)
	cmd.Stderr = tail
	cmd.Stdout = io.Discard
	if err := cmd.Start(); err != nil {
		toChromeR.Close()
		toChromeW.Close()
		fromChromeR.Close()
		fromChromeW.Close()
		return nil, fmt.Errorf("start chromium: %w", err)
	}
	toChromeR.Close()
	fromChromeW.Close()

	b := &browser{
		cmd:       cmd,
		in:        toChromeW,
		out:       bufio.NewReaderSize(fromChromeR, 1<<20),
		ioTimeout: 30 * time.Second,
		stderr:    tail,
	}
	if logf != nil {
		logf("douyin browser started", "chrome", chrome)
	}
	if err := b.open(); err != nil {
		b.Close()
		return nil, err
	}
	return b, nil
}

// open creates a page target and attaches to it.
func (b *browser) open() error {
	var created struct {
		TargetID string `json:"targetId"`
	}
	raw, err := b.call("", "Target.createTarget", map[string]any{"url": "about:blank"})
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &created); err != nil || created.TargetID == "" {
		return fmt.Errorf("createTarget: %s", string(raw))
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	raw, err = b.call("", "Target.attachToTarget", map[string]any{"targetId": created.TargetID, "flatten": true})
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &attached); err != nil || attached.SessionID == "" {
		return fmt.Errorf("attachToTarget: %s", string(raw))
	}
	b.session = attached.SessionID
	return nil
}

// call sends one CDP command and waits for the matching reply. Events (messages
// without our id) are skipped.
func (b *browser) call(session, method string, params any) (json.RawMessage, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.dead.Load() {
		return nil, errors.New("browser is closed")
	}
	b.seq++
	id := b.seq
	msg := cdpMessage{ID: id, Method: method, Params: params, SessionID: session}
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	data = append(data, 0)
	if _, err := b.in.Write(data); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(b.ioTimeout)
	for {
		type readResult struct {
			line []byte
			err  error
		}
		ch := make(chan readResult, 1)
		go func() {
			line, err := b.out.ReadBytes(0)
			ch <- readResult{line: line, err: err}
		}()
		var res readResult
		select {
		case res = <-ch:
		case <-time.After(time.Until(deadline)):
			// A half-read frame would desynchronise every later call; the
			// cheapest correct recovery is to drop this browser.
			b.dead.Store(true)
			go b.Close()
			return nil, fmt.Errorf("%s: devtools read timed out after %s", method, b.ioTimeout)
		}
		if res.err != nil {
			// The pipe broke (Chromium exited or crashed): nothing more can
			// be read from it, so let the client start a new browser.
			b.dead.Store(true)
			go b.Close()
			return nil, res.err
		}
		var reply cdpMessage
		if err := json.Unmarshal(res.line[:len(res.line)-1], &reply); err != nil {
			continue
		}
		if reply.ID != id {
			continue
		}
		if reply.Error != nil {
			return nil, fmt.Errorf("%s: %s", method, reply.Error.Message)
		}
		return reply.Result, nil
	}
}

// evaluate runs a JavaScript expression in the page and returns its value.
// When await is set the expression must return a promise.
func (b *browser) evaluate(expr string, await bool, timeoutMS int) (string, error) {
	params := map[string]any{
		"expression":    expr,
		"returnByValue": true,
	}
	if await {
		params["awaitPromise"] = true
	}
	if timeoutMS > 0 {
		params["timeout"] = timeoutMS
	}
	raw, err := b.call(b.session, "Runtime.evaluate", params)
	if err != nil {
		return "", err
	}
	var out struct {
		Result struct {
			Type  string          `json:"type"`
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if out.ExceptionDetails != nil {
		return "", fmt.Errorf("javascript error: %s", out.ExceptionDetails.Text)
	}
	if len(out.Result.Value) == 0 {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(out.Result.Value, &s); err == nil {
		return s, nil
	}
	return string(out.Result.Value), nil
}

// isDead reports whether the browser must be replaced.
func (b *browser) isDead() bool { return b.dead.Load() }

// navigate loads url in the attached page.
func (b *browser) navigate(url string) error {
	_, err := b.call(b.session, "Page.navigate", map[string]any{"url": url})
	return err
}

// Close stops Chromium and releases the pipes.
func (b *browser) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	b.dead.Store(true)
	b.mu.Unlock()
	if b.in != nil {
		_ = b.in.Close()
	}
	if b.cmd != nil && b.cmd.Process != nil {
		_ = b.cmd.Process.Kill()
		_, _ = b.cmd.Process.Wait()
	}
}

// tailBuffer keeps the last n bytes written to it, for error reporting.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func newTailBuffer(max int) *tailBuffer { return &tailBuffer{max: max} }

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

// waitFor runs check until it reports done or the context ends.
func waitFor(ctx context.Context, every time.Duration, check func() (bool, error)) error {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	var lastErr error
	for {
		ok, err := check()
		if err == nil && ok {
			return nil
		}
		if err != nil {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("%w (last error: %v)", ctx.Err(), lastErr)
			}
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
