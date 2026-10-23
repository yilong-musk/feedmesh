package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

func TestParseArgsAcceptsGlobalFlagsAnywhere(t *testing.T) {
	cmd, pos, o, err := parseArgs([]string{
		"fetch", "2803301701", "--json", "--limit", "5", "--timeout", "2s", "-o", "-",
	})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if cmd != "fetch" {
		t.Fatalf("cmd = %q", cmd)
	}
	if len(pos) != 1 || pos[0] != "2803301701" {
		t.Fatalf("pos = %v", pos)
	}
	if !o.json || o.limit != 5 || o.timeout != 2*time.Second || o.output != "-" {
		t.Fatalf("options = %+v", o)
	}

	cmd, pos, o, err = parseArgs([]string{"--json", "fetch", "rmrb"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if cmd != "fetch" || len(pos) != 1 || !o.json {
		t.Fatalf("flags before the command broke parsing: cmd=%q pos=%v json=%v", cmd, pos, o.json)
	}
}

func TestParseArgsPurgeFlags(t *testing.T) {
	cmd, pos, o, err := parseArgs([]string{"cache", "purge", "--uid", "2803301701", "--dry-run", "--json", "--quiet"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if cmd != "cache" || len(pos) != 1 || pos[0] != "purge" {
		t.Fatalf("cmd=%q pos=%v", cmd, pos)
	}
	if o.uid != "2803301701" || !o.dryRun || !o.json || !o.quiet {
		t.Fatalf("options = %+v", o)
	}
}

func TestParseArgsRejectsBadInput(t *testing.T) {
	cases := [][]string{
		{"fetch", "--nope"},
		{"fetch", "--limit", "abc"},
		{"fetch", "--limit", "-3"},
		{"fetch", "--timeout", "soon"},
		{"fetch", "--json=1"},
		{"fetch", "-o"},
	}
	for _, args := range cases {
		if _, _, _, err := parseArgs(args); err == nil {
			t.Errorf("parseArgs(%v) = nil error, want a usage error", args)
		} else if code := weibo.CodeOf(err); code != weibo.CodeBadRequest {
			t.Errorf("parseArgs(%v) code = %q, want BAD_REQUEST", args, code)
		}
	}
}

func TestParseDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"9s":    9 * time.Second,
		"500ms": 500 * time.Millisecond,
		"1ms":   time.Millisecond,
		"2":     2 * time.Second,
	}
	for in, want := range cases {
		got, err := parseDuration(in)
		if err != nil {
			t.Errorf("parseDuration(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseDuration(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := parseDuration("soon"); err == nil {
		t.Error("expected an error for garbage")
	}
}

func TestExitCodeMapping(t *testing.T) {
	cases := map[string]int{
		weibo.CodeBadRequest:          exitUsage,
		weibo.CodeNetwork:             exitNetwork,
		weibo.CodeTimeout:             exitNetwork,
		weibo.CodeUpstreamUnavailable: exitNetwork,
		weibo.CodeUpstreamBlocked:     exitBlocked,
		weibo.CodeUserNotFound:        exitNotFound,
		weibo.CodeLoginRequired:       exitNotFound,
		weibo.CodeBreakerOpen:         exitBreakerOpen,
		"WHATEVER":                    exitOther,
	}
	for code, want := range cases {
		if got := exitCodeFor(code); got != want {
			t.Errorf("exitCodeFor(%q) = %d, want %d", code, got, want)
		}
	}
}

func TestVersionJSONIsOneLine(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run([]string{"version", "--json"}, &out, &errBuf); code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errBuf.String())
	}
	if strings.Count(out.String(), "\n") != 1 {
		t.Fatalf("stdout must be exactly one line, got %q", out.String())
	}
	var got versionInfo
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v (%q)", err, out.String())
	}
	if got.Version == "" || got.Go == "" || got.OS == "" {
		t.Fatalf("version payload = %+v", got)
	}
}

func TestHelpExitsZero(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run([]string{"help"}, &out, &errBuf); code != exitOK {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"USAGE", "EXIT CODES", "cache purge --all", "--dry-run", "--quiet"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("help text is missing %q", want)
		}
	}
}

func TestUsageErrorGoesToStdoutAsJSONAndStderrAsLog(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"fetch", "--json", "--cache-dir", t.TempDir()}, &out, &errBuf)
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
	if strings.Count(out.String(), "\n") != 1 {
		t.Fatalf("stdout must be a single JSON line, got %q", out.String())
	}
	var env errorEnvelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not JSON: %v (%q)", err, out.String())
	}
	if env.OK || env.Error.Code != weibo.CodeBadRequest || env.Error.Message == "" {
		t.Fatalf("envelope = %+v", env)
	}
	if !strings.Contains(errBuf.String(), `"level":"error"`) {
		t.Fatalf("stderr is not a structured log line: %q", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), `"msg"`) {
		t.Fatalf("stderr log line lacks msg: %q", errBuf.String())
	}
}

func TestQuietSilencesStderr(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"fetch", "--quiet", "--json", "--cache-dir", t.TempDir()}, &out, &errBuf)
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
	if errBuf.Len() != 0 {
		t.Fatalf("--quiet must silence stderr, got %q", errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatal("--json must still emit the error envelope on stdout")
	}
}

func TestQuietSurvivesFlagParseError(t *testing.T) {
	// A bad flag used to rebuild options from scratch and drop quiet, which
	// leaked a log line to stderr on the one path agents rely on for silence.
	for _, quietFlag := range []string{"--quiet", "-q"} {
		var out, errBuf bytes.Buffer
		code := run([]string{"fetch", quietFlag, "--json", "--nope"}, &out, &errBuf)
		if code != exitUsage {
			t.Fatalf("%s: exit = %d, want %d", quietFlag, code, exitUsage)
		}
		if errBuf.Len() != 0 {
			t.Fatalf("%s: stderr must stay silent on the parse-error path, got %q", quietFlag, errBuf.String())
		}
		if strings.Count(out.String(), "\n") != 1 {
			t.Fatalf("%s: stdout must be one JSON line, got %q", quietFlag, out.String())
		}
		var env errorEnvelope
		if err := json.Unmarshal(out.Bytes(), &env); err != nil {
			t.Fatalf("%s: stdout is not JSON: %v", quietFlag, err)
		}
		if env.OK || env.Error.Code != weibo.CodeBadRequest {
			t.Fatalf("%s: envelope = %+v", quietFlag, env)
		}
	}
}

func seedCache(t *testing.T, dir string, keys ...string) *cache.Cache {
	t.Helper()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		if err := c.Set(k, map[string]string{"k": k}, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func TestCachePurgeDryRunDeletesNothing(t *testing.T) {
	dir := t.TempDir()
	c := seedCache(t, dir, "profile:2803301701", "feed:2803301701:10:false", "longtext:1001")

	var out, errBuf bytes.Buffer
	code := run([]string{"cache", "purge", "--all", "--dry-run", "--json", "--cache-dir", dir}, &out, &errBuf)
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errBuf.String())
	}
	var result purgeResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not JSON: %v (%q)", err, out.String())
	}
	if !result.DryRun || result.Entries != 3 || result.Bytes == 0 || result.Deleted != 0 {
		t.Fatalf("dry-run result = %+v", result)
	}
	if st, _ := c.Stat(); st.Entries != 3 {
		t.Fatalf("dry run deleted entries: %d left", st.Entries)
	}

	// The real purge must then report and remove all three.
	out.Reset()
	code = run([]string{"cache", "purge", "--all", "--json", "--cache-dir", dir}, &out, &errBuf)
	if code != exitOK {
		t.Fatalf("exit = %d", code)
	}
	var purged purgeResult
	if err := json.Unmarshal(out.Bytes(), &purged); err != nil {
		t.Fatal(err)
	}
	if purged.Deleted != 3 || purged.DryRun {
		t.Fatalf("purge result = %+v", purged)
	}
	if st, _ := c.Stat(); st.Entries != 0 {
		t.Fatalf("entries left = %d", st.Entries)
	}
}

func TestCachePurgeUIDIsPrecise(t *testing.T) {
	dir := t.TempDir()
	c := seedCache(t, dir,
		"profile:2803301701",
		"profile:28033017011", // a different uid that shares a prefix
		"timeline:2803301701",
		"feed:2803301701:10:false",
		"longtext:1001",
	)

	var out, errBuf bytes.Buffer
	code := run([]string{"cache", "purge", "--uid", "2803301701", "--dry-run", "--json", "--cache-dir", dir}, &out, &errBuf)
	if code != exitOK {
		t.Fatalf("exit = %d", code)
	}
	var result purgeResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Entries != 3 {
		t.Fatalf("dry-run matched %d entries, want 3 (profile, timeline, feed)", result.Entries)
	}
	if st, _ := c.Stat(); st.Entries != 5 {
		t.Fatalf("dry run deleted something: %d entries", st.Entries)
	}

	out.Reset()
	if code := run([]string{"cache", "purge", "--uid", "2803301701", "--json", "--cache-dir", dir}, &out, &errBuf); code != exitOK {
		t.Fatalf("exit = %d", code)
	}
	var purged purgeResult
	if err := json.Unmarshal(out.Bytes(), &purged); err != nil {
		t.Fatal(err)
	}
	if purged.Deleted != 3 {
		t.Fatalf("deleted = %d, want 3", purged.Deleted)
	}
	list, err := c.List()
	if err != nil {
		t.Fatal(err)
	}
	left := make([]string, 0, len(list))
	for _, ki := range list {
		left = append(left, ki.Key)
	}
	for _, want := range []string{"profile_28033017011", "longtext_1001"} {
		found := false
		for _, k := range left {
			if k == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%q was removed but should have survived: %v", want, left)
		}
	}
}

func TestCachePurgeRequiresScope(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"cache", "purge", "--json", "--cache-dir", t.TempDir()}, &out, &errBuf)
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(out.String(), weibo.CodeBadRequest) {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestCacheStatPlainText(t *testing.T) {
	dir := t.TempDir()
	seedCache(t, dir, "profile:1")

	var out, errBuf bytes.Buffer
	if code := run([]string{"cache", "stat", "--cache-dir", dir}, &out, &errBuf); code != exitOK {
		t.Fatalf("exit = %d", code)
	}
	line := out.String()
	if !strings.Contains(line, "entries=1") || !strings.Contains(line, "dir=") {
		t.Fatalf("stat output = %q", line)
	}
	if strings.Contains(line, "{") {
		t.Fatalf("plain output must not be JSON: %q", line)
	}
}
