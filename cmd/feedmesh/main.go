// Command feedmesh turns a weibo timeline into RSS.
//
// Output discipline: stdout carries data only (RSS XML or exactly one line of
// JSON); stderr carries newline-delimited JSON logs; the process never reads
// stdin, never prompts, and never colours its output.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/feed"
	"github.com/yilong-musk/feedmesh/internal/guard"
	"github.com/yilong-musk/feedmesh/internal/httpapi"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/reddit"
	"github.com/yilong-musk/feedmesh/internal/smzdm"
	"github.com/yilong-musk/feedmesh/internal/sspai"
	"github.com/yilong-musk/feedmesh/internal/v2ex"
	"github.com/yilong-musk/feedmesh/internal/weibo"
	"github.com/yilong-musk/feedmesh/internal/xnitter"
	"github.com/yilong-musk/feedmesh/internal/youtube"
)

// version is injected at build time with -X main.version=...
var version = "dev"

const (
	defaultListen         = "127.0.0.1:3000"
	defaultTimeout        = weibo.DefaultTimeout
	defaultSspaiTimeout   = 15 * time.Second
	defaultV2exTimeout    = 15 * time.Second
	defaultSmzdmTimeout   = 15 * time.Second
	defaultXTimeout       = 20 * time.Second
	maxXTimeout           = 25 * time.Second
	defaultYouTubeTimeout = 20 * time.Second
	defaultRedditTimeout  = 15 * time.Second
	breakerCooloff        = 10 * time.Minute
)

// Exit codes, fixed by the CLI contract.
const (
	exitOK          = 0
	exitUsage       = 2
	exitNetwork     = 3
	exitBlocked     = 4
	exitNotFound    = 5
	exitBreakerOpen = 6
	exitOther       = 10
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// options holds every flag the CLI understands. Flags are global: the parser
// accepts them before or after the subcommand.
type options struct {
	json            bool
	quiet           bool
	refresh         bool
	all             bool
	dryRun          bool
	help            bool
	timeout         time.Duration
	timeoutSet      bool
	cacheDir        string
	cacheDirCleanup func()
	listen          string
	limit           int
	output          string
	uid             string
	handle          string
	token           string
	rateLimit       int
	rateLimitSet    bool
}

func run(args []string, stdout, stderr io.Writer) int {
	cmd, pos, o, err := parseArgs(args)
	if err != nil {
		// --json and --quiet may still have been requested before the bad flag;
		// honour both so an agent's "quiet" contract holds on the error path too.
		o = &options{
			json:  hasFlag(args, "--json"),
			quiet: hasFlag(args, "--quiet") || hasFlag(args, "-q"),
		}
		logger := logx.New(stderr)
		if o.quiet {
			logger = logx.Discard()
		}
		return fail(o, logger, stdout, err)
	}

	logger := logx.New(stderr)
	if o.quiet {
		logger = logx.Discard()
	}
	applyEnv(o)
	if o.cacheDirCleanup != nil {
		defer o.cacheDirCleanup()
	}

	if o.help || cmd == "help" || cmd == "" {
		if err := printHelp(stdout); err != nil {
			logger.Error("help output failed", "error", err.Error())
			return exitOther
		}
		return exitOK
	}

	switch cmd {
	case "version":
		return cmdVersion(o, stdout)
	case "serve":
		return cmdServe(o, stdout, logger)
	case "fetch":
		return cmdFetch(pos, o, stdout, logger)
	case "doctor":
		return cmdDoctor(o, stdout, logger)
	case "cache":
		return cmdCache(pos, o, stdout, logger)
	default:
		return fail(o, logger, stdout, usageError("unknown command %q", cmd))
	}
}

// ---------------------------------------------------------------- flag parsing

func parseArgs(args []string) (string, []string, *options, error) {
	o := &options{timeout: defaultTimeout}
	var cmd string
	var pos []string

	for i := 0; i < len(args); i++ {
		a := args[i]

		if a == "--" {
			for _, rest := range args[i+1:] {
				if cmd == "" {
					cmd = rest
					continue
				}
				pos = append(pos, rest)
			}
			break
		}

		switch {
		case a == "-h" || a == "--help":
			o.help = true
			continue
		case a == "-o" || a == "--output":
			if i+1 >= len(args) {
				return "", nil, nil, usageError("flag %s needs a value", a)
			}
			i++
			o.output = args[i]
			continue
		case len(a) > 1 && a[0] == '-':
			name, value, hasValue := a, "", false
			if eq := strings.IndexByte(a, '='); eq > 1 {
				name, value, hasValue = a[:eq], a[eq+1:], true
			}

			boolFlag := func(dst *bool) error {
				if hasValue {
					return usageError("flag %s takes no value", name)
				}
				*dst = true
				return nil
			}
			strFlag := func(dst *string) error {
				if !hasValue {
					if i+1 >= len(args) {
						return usageError("flag %s needs a value", name)
					}
					i++
					value = args[i]
				}
				*dst = value
				return nil
			}

			var err error
			switch name {
			case "--json":
				err = boolFlag(&o.json)
			case "--quiet", "-q":
				err = boolFlag(&o.quiet)
			case "--refresh":
				err = boolFlag(&o.refresh)
			case "--all":
				err = boolFlag(&o.all)
			case "--dry-run":
				err = boolFlag(&o.dryRun)
			case "--cache-dir":
				err = strFlag(&o.cacheDir)
			case "--listen":
				err = strFlag(&o.listen)
			case "--output", "-o":
				err = strFlag(&o.output)
			case "--uid":
				err = strFlag(&o.uid)
			case "--handle":
				err = strFlag(&o.handle)
			case "--token":
				err = strFlag(&o.token)
			case "--rate-limit":
				var raw string
				if err = strFlag(&raw); err == nil {
					n, convErr := strconv.Atoi(strings.TrimSpace(raw))
					if convErr != nil || n < 0 {
						err = usageError("flag --rate-limit needs a non-negative integer, got %q", raw)
					} else {
						o.rateLimit = n
						o.rateLimitSet = true
					}
				}
			case "--limit":
				var raw string
				if err = strFlag(&raw); err == nil {
					n, convErr := strconv.Atoi(strings.TrimSpace(raw))
					if convErr != nil || n < 0 {
						err = usageError("flag --limit needs a non-negative integer, got %q", raw)
					} else {
						o.limit = n
					}
				}
			case "--timeout":
				var raw string
				if err = strFlag(&raw); err == nil {
					d, convErr := parseDuration(raw)
					if convErr != nil {
						err = usageError("flag --timeout needs a duration such as 9s or 500ms, got %q", raw)
					} else {
						o.timeout = d
						o.timeoutSet = true
					}
				}
			default:
				err = usageError("unknown flag %q", name)
			}
			if err != nil {
				return "", nil, nil, err
			}
			continue
		default:
			if cmd == "" {
				cmd = a
				continue
			}
			pos = append(pos, a)
		}
	}
	return cmd, pos, o, nil
}

func parseDuration(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("empty duration")
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return d, nil
	}
	// Bare numbers are seconds, a common slip worth accepting.
	if n, err := strconv.ParseFloat(raw, 64); err == nil {
		return time.Duration(n * float64(time.Second)), nil
	}
	return 0, fmt.Errorf("invalid duration %q", raw)
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag || strings.HasPrefix(a, flag+"=") {
			return true
		}
	}
	return false
}

// envFirst returns the first non-empty environment value among names.
func envFirst(names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(os.Getenv(n)); v != "" {
			return v
		}
	}
	return ""
}

// applyEnv resolves the flag > env > default precedence.
func applyEnv(o *options) {
	if o.token == "" {
		o.token = envFirst("FEEDMESH_TOKEN")
	}
	if !o.rateLimitSet {
		if raw := envFirst("FEEDMESH_RATE_LIMIT"); raw != "" {
			if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && n >= 0 {
				o.rateLimit = n
				o.rateLimitSet = true
			}
		}
	}
	if o.cacheDir == "" {
		// Older names keep working: weibo-rss-pro and the original weibo-rss.
		o.cacheDir = envFirst("FEEDMESH_CACHE", "WEIBO_RSS_PRO_CACHE", "WEIBO_RSS_CACHE")
	}
	if o.cacheDir == "" {
		o.cacheDir, o.cacheDirCleanup = defaultCacheDir()
	}
	if o.listen == "" {
		o.listen = envFirst("FEEDMESH_LISTEN", "WEIBO_RSS_PRO_LISTEN", "WEIBO_RSS_LISTEN")
	}
	if o.listen == "" {
		o.listen = defaultListen
	}
	if o.timeout <= 0 {
		o.timeout = defaultTimeout
	}
}

func defaultCacheDir() (string, func()) {
	if dir, err := os.UserCacheDir(); err == nil && dir != "" {
		return filepath.Join(dir, "feedmesh"), nil
	}
	// A predictable directory under /tmp is shared with every local user. If
	// the platform cannot give us a home cache, use a private, random directory
	// instead; losing the cache across restarts is preferable to cache
	// poisoning. Remove it on exit so repeated runs cannot leak directories.
	dir, err := os.MkdirTemp("", "feedmesh-cache-*")
	if err != nil {
		return "", nil
	}
	return dir, func() { _ = os.RemoveAll(dir) }
}

// ------------------------------------------------------------------- plumbing

func newService(o *options, logger *logx.Logger) (*feed.Service, *cache.Cache, *weibo.Client, *guard.Breaker, error) {
	c, err := cache.New(o.cacheDir)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	// This is a read/write command (serve/fetch/doctor), so sweeping expired
	// entries here is safe. The cache CLI uses New directly to keep dry-run
	// strictly read-only.
	_, _ = c.GC()
	breaker := guard.NewBreaker(breakerCooloff)
	client := weibo.New(weibo.Config{
		Timeout: o.timeout,
		Breaker: breaker,
		Log:     logger,
		Retries: 3,
		Backoff: 300 * time.Millisecond,
	})
	sspaiTimeout, v2exTimeout, xTimeout, smzdmTimeout, youtubeTimeout, redditTimeout := providerTimeouts(o)
	sp := sspai.New(sspai.Config{Timeout: sspaiTimeout})
	vx := v2ex.New(v2ex.Config{Timeout: v2exTimeout})
	xn := xnitter.New(xnitter.Config{Timeout: xTimeout})
	sz := smzdm.New(smzdm.Config{Timeout: smzdmTimeout})
	yt := youtube.New(youtube.Config{Timeout: youtubeTimeout})
	rd := reddit.New(reddit.Config{Timeout: redditTimeout, Log: logger})
	svc := feed.NewService(feed.Clients{Weibo: client, Sspai: sp, V2ex: vx, X: xn, Smzdm: sz, Youtube: yt, Reddit: rd}, c, logger, feed.DefaultTTLs())
	return svc, c, client, breaker, nil
}

// providerTimeouts resolves the per-provider request timeouts. X is capped even
// when --timeout is larger, so a single X feed request cannot spend the whole
// handler deadline on slow mirror attempts.
func providerTimeouts(o *options) (sspaiTimeout, v2exTimeout, xTimeout, smzdmTimeout, youtubeTimeout, redditTimeout time.Duration) {
	sspaiTimeout = defaultSspaiTimeout
	v2exTimeout = defaultV2exTimeout
	xTimeout = defaultXTimeout
	smzdmTimeout = defaultSmzdmTimeout
	youtubeTimeout = defaultYouTubeTimeout
	redditTimeout = defaultRedditTimeout
	if o.timeoutSet {
		sspaiTimeout = o.timeout
		v2exTimeout = o.timeout
		xTimeout = o.timeout
		smzdmTimeout = o.timeout
		youtubeTimeout = o.timeout
		redditTimeout = o.timeout
	}
	if sspaiTimeout <= 0 {
		sspaiTimeout = defaultTimeout
	}
	if v2exTimeout <= 0 {
		v2exTimeout = defaultTimeout
	}
	if xTimeout <= 0 {
		xTimeout = defaultTimeout
	}
	if smzdmTimeout <= 0 {
		smzdmTimeout = defaultTimeout
	}
	if xTimeout > maxXTimeout {
		xTimeout = maxXTimeout
	}
	if youtubeTimeout <= 0 {
		youtubeTimeout = defaultTimeout
	}
	if redditTimeout <= 0 {
		redditTimeout = defaultTimeout
	}
	return sspaiTimeout, v2exTimeout, xTimeout, smzdmTimeout, youtubeTimeout, redditTimeout
}

func exitCodeFor(code string) int {
	switch code {
	case weibo.CodeBadRequest:
		return exitUsage
	case weibo.CodeNetwork, weibo.CodeTimeout, weibo.CodeUpstreamUnavailable:
		return exitNetwork
	case weibo.CodeUpstreamBlocked:
		return exitBlocked
	case weibo.CodeUserNotFound, weibo.CodeLoginRequired:
		return exitNotFound
	case weibo.CodeBreakerOpen:
		return exitBreakerOpen
	default:
		return exitOther
	}
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	OK    bool         `json:"ok"`
	Error errorPayload `json:"error"`
}

func usageError(format string, args ...any) error {
	return weibo.NewError(weibo.CodeBadRequest, 0, format, args...)
}

// fail reports an error on both channels and returns the mapped exit code.
func fail(o *options, logger *logx.Logger, stdout io.Writer, err error) int {
	code := weibo.CodeOf(err)
	logger.Error(err.Error(), "code", code)
	if o != nil && o.json {
		_ = writeJSONLine(stdout, errorEnvelope{
			OK:    false,
			Error: errorPayload{Code: code, Message: err.Error()},
		})
	}
	return exitCodeFor(code)
}

func writeJSONLine(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}

// ------------------------------------------------------------------- commands

type versionInfo struct {
	Version string `json:"version"`
	Go      string `json:"go"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

func cmdVersion(o *options, stdout io.Writer) int {
	info := versionInfo{Version: version, Go: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH}
	if o.json {
		if err := writeJSONLine(stdout, info); err != nil {
			return exitOther
		}
		return exitOK
	}
	if _, err := fmt.Fprintf(stdout, "feedmesh %s (%s/%s, %s)\n", info.Version, info.OS, info.Arch, info.Go); err != nil {
		return exitOther
	}
	return exitOK
}

type fetchJSON struct {
	OK          bool        `json:"ok"`
	UID         string      `json:"uid"`
	ScreenName  string      `json:"screen_name"`
	Description string      `json:"description"`
	FetchedAt   time.Time   `json:"fetched_at"`
	Cached      bool        `json:"cached"`
	Count       int         `json:"count"`
	Items       []feed.Item `json:"items"`
}

func cmdFetch(pos []string, o *options, stdout io.Writer, logger *logx.Logger) int {
	if len(pos) == 0 {
		return fail(o, logger, stdout, usageError("fetch needs a target: a numeric uid or a vanity domain"))
	}
	if len(pos) > 1 {
		return fail(o, logger, stdout, usageError("fetch takes exactly one target, got %d", len(pos)))
	}

	svc, _, _, _, err := newService(o, logger)
	if err != nil {
		return fail(o, logger, stdout, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), o.timeout*8)
	defer cancel()

	target := strings.TrimSpace(pos[0])
	uid, err := svc.ResolveUID(ctx, target, o.refresh)
	if err != nil {
		return fail(o, logger, stdout, err)
	}
	if uid != target {
		logger.Info("resolved domain to uid", "target", target, "uid", uid)
	}

	f, err := svc.GetFeed(ctx, uid, o.limit, o.refresh)
	if err != nil {
		return fail(o, logger, stdout, err)
	}
	logger.Info("feed built", "uid", f.UID, "items", len(f.Items), "cached", f.Cached)

	var payload []byte
	if o.json {
		buf := &strings.Builder{}
		if err := writeJSONLine(buf, fetchJSON{
			OK:          true,
			UID:         f.UID,
			ScreenName:  f.ScreenName,
			Description: f.Description,
			FetchedAt:   f.FetchedAt,
			Cached:      f.Cached,
			Count:       len(f.Items),
			Items:       f.Items,
		}); err != nil {
			return fail(o, logger, stdout, err)
		}
		payload = []byte(buf.String())
	} else {
		payload = feed.RenderRSS(f)
	}

	if o.output != "" && o.output != "-" {
		if err := os.WriteFile(o.output, payload, 0o644); err != nil {
			return fail(o, logger, stdout, fmt.Errorf("write output file: %w", err))
		}
		logger.Info("wrote output", "path", o.output, "bytes", len(payload))
		return exitOK
	}
	if _, err := stdout.Write(payload); err != nil {
		return fail(o, logger, stdout, err)
	}
	return exitOK
}

type doctorJSON struct {
	OK    bool         `json:"ok"`
	Steps []weibo.Step `json:"steps"`
}

func cmdDoctor(o *options, stdout io.Writer, logger *logx.Logger) int {
	_, _, client, _, err := newService(o, logger)
	if err != nil {
		return fail(o, logger, stdout, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), o.timeout*8)
	defer cancel()

	steps := client.Probe(ctx, strings.TrimSpace(o.uid))
	ok := true
	failed := ""
	for _, st := range steps {
		if !st.OK {
			ok = false
			if failed == "" {
				failed = st.Code
			}
		}
	}

	if o.json {
		if err := writeJSONLine(stdout, doctorJSON{OK: ok, Steps: steps}); err != nil {
			return exitOther
		}
	} else {
		for _, st := range steps {
			status := "ok"
			if !st.OK {
				status = "fail"
			}
			line := fmt.Sprintf("%-12s %-4s %5dms http=%d", st.Step, status, st.MS, st.HTTPStatus)
			if st.Error != "" {
				line += "  " + st.Error
			}
			if _, err := fmt.Fprintln(stdout, line); err != nil {
				logger.Error("doctor output failed", "error", err.Error())
				return exitOther
			}
		}
	}

	if ok {
		return exitOK
	}
	if failed == "" {
		failed = weibo.CodeInternal
	}
	return exitCodeFor(failed)
}

type purgeResult struct {
	DryRun  bool   `json:"dry_run"`
	Scope   string `json:"scope"`
	Deleted int    `json:"deleted"`
	Entries int    `json:"entries"`
	Bytes   int64  `json:"bytes"`
}

func cmdCache(pos []string, o *options, stdout io.Writer, logger *logx.Logger) int {
	if len(pos) == 0 {
		return fail(o, logger, stdout, usageError("cache needs a subcommand: stat or purge"))
	}
	c, err := cache.New(o.cacheDir)
	if err != nil {
		return fail(o, logger, stdout, err)
	}
	writeErr := func(err error) int {
		logger.Error("cache output failed", "error", err.Error())
		return exitOther
	}

	switch pos[0] {
	case "stat":
		_, _ = c.GC()
		st, err := c.Stat()
		if err != nil {
			return fail(o, logger, stdout, err)
		}
		if o.json {
			if err := writeJSONLine(stdout, st); err != nil {
				return writeErr(err)
			}
			return exitOK
		}
		if _, err := fmt.Fprintf(stdout, "dir=%s entries=%d bytes=%d\n", st.Dir, st.Entries, st.Bytes); err != nil {
			return writeErr(err)
		}
		return exitOK

	case "purge":
		uid := strings.TrimSpace(o.uid)
		handle := strings.TrimSpace(o.handle)
		scopes := 0
		if o.all {
			scopes++
		}
		if uid != "" {
			scopes++
		}
		if handle != "" {
			scopes++
		}
		if scopes == 0 {
			return fail(o, logger, stdout, usageError("cache purge needs --all, --uid <uid> or --handle <handle>"))
		}
		if scopes > 1 {
			return fail(o, logger, stdout, usageError("cache purge accepts only one of --all, --uid or --handle"))
		}

		scope := "all"
		if uid != "" {
			if err := weibo.ValidateUID(uid); err != nil {
				return fail(o, logger, stdout, err)
			}
			scope = "uid:" + uid
		}
		if handle != "" {
			h, err := xnitter.NormalizeHandle(handle)
			if err != nil {
				return fail(o, logger, stdout, usageError("%v", err))
			}
			handle = h
			scope = "handle:" + handle
		}

		list, err := c.ListManaged()
		if err != nil {
			return fail(o, logger, stdout, err)
		}
		byKey := make(map[string]cache.KeyInfo, len(list))
		for _, ki := range list {
			byKey[ki.Key] = ki
		}
		targets := make(map[string]cache.KeyInfo)
		add := func(key string) {
			if key == "" {
				return
			}
			if ki, ok := byKey[key]; ok {
				targets[key] = ki
			}
		}

		switch {
		case o.all:
			for _, ki := range list {
				add(ki.Key)
			}
		case uid != "":
			add("profile_" + uid)
			add("timeline_" + uid)
			for _, ki := range list {
				if strings.HasPrefix(ki.Key, "feed_"+uid+"_") ||
					strings.HasPrefix(ki.Key, "longtext_"+uid+"_") {
					add(ki.Key)
				}
				if strings.HasPrefix(ki.Key, "domain_") {
					var cachedUID string
					if hit, err := c.PeekStored(ki.Key, &cachedUID); err == nil && hit && cachedUID == uid {
						add(ki.Key)
					}
				}
			}
			// Long-text entries are keyed by status id, so recover the ids
			// from the timeline we are about to delete. PeekStored ignores the
			// TTL: an expired timeline still names long-text entries that must
			// be removed by this targeted purge.
			var tl weibo.Timeline
			if hit, err := c.PeekStored("timeline:"+uid, &tl); err == nil && hit {
				for _, id := range longTextIDs(tl.Items) {
					add("longtext_" + id)
				}
			}
		case handle != "":
			add("x_" + strings.ToLower(handle))
		}

		var bytes int64
		for _, ki := range targets {
			bytes += ki.Bytes
		}
		result := purgeResult{Scope: scope, DryRun: o.dryRun, Entries: len(targets), Bytes: bytes}

		// A dry run reports the blast radius and touches nothing.
		if o.dryRun {
			if o.json {
				if err := writeJSONLine(stdout, result); err != nil {
					return writeErr(err)
				}
				return exitOK
			}
			if _, err := fmt.Fprintf(stdout, "dry-run scope=%s would_delete=%d bytes=%d\n", scope, result.Entries, result.Bytes); err != nil {
				return writeErr(err)
			}
			return exitOK
		}

		for key := range targets {
			ok, err := c.Delete(key)
			if err != nil {
				return fail(o, logger, stdout, err)
			}
			if ok {
				result.Deleted++
			}
		}
		logger.Info("cache purged", "scope", scope, "deleted", result.Deleted)
		if o.json {
			if err := writeJSONLine(stdout, result); err != nil {
				return writeErr(err)
			}
			return exitOK
		}
		if _, err := fmt.Fprintf(stdout, "purged scope=%s deleted=%d\n", scope, result.Deleted); err != nil {
			return writeErr(err)
		}
		return exitOK

	default:
		return fail(o, logger, stdout, usageError("unknown cache subcommand %q", pos[0]))
	}
}

// longTextIDs walks a timeline (including quoted posts) and collects the status
// ids whose long-text cache entries belong to it.
func longTextIDs(items []*weibo.MBlog) []string {
	seen := map[string]bool{}
	var ids []string
	var walk func(*weibo.MBlog, int)
	walk = func(m *weibo.MBlog, depth int) {
		if m == nil || depth > 32 {
			return
		}
		if m.IsLongText {
			if id := m.IDString(); id != "" && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		walk(m.Retweeted, depth+1)
	}
	for _, m := range items {
		walk(m, 0)
	}
	return ids
}

func listenIsLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// cmdServe runs the HTTP service until SIGINT/SIGTERM.
func cmdServe(o *options, stdout io.Writer, logger *logx.Logger) int {
	svc, c, client, breaker, err := newService(o, logger)
	if err != nil {
		return fail(o, logger, stdout, err)
	}

	rateLimit := o.rateLimit
	loopback := listenIsLoopback(o.listen)
	if !loopback {
		if o.token == "" {
			logger.Warn("listening on a non-loopback address without FEEDMESH_TOKEN; anyone who can reach this port can read feeds and trigger upstream requests")
		}
		if rateLimit == 0 && !o.rateLimitSet {
			rateLimit = 120
		}
	}
	api := httpapi.New(httpapi.Config{
		Version:        version,
		Service:        svc,
		Cache:          c,
		Client:         client,
		Breaker:        breaker,
		Log:            logger,
		Token:          o.token,
		RateLimit:      rateLimit,
		RequestTimeout: 3 * time.Minute,
	})

	ln, err := net.Listen("tcp", o.listen)
	if err != nil {
		return fail(o, logger, stdout, fmt.Errorf("listen on %s: %w", o.listen, err))
	}

	// The readiness line is the contract: one line of JSON on stdout, flushed
	// immediately so supervisors never have to sleep and guess.
	ready := struct {
		Event   string `json:"event"`
		Addr    string `json:"addr"`
		Version string `json:"version"`
	}{Event: "ready", Addr: ln.Addr().String(), Version: version}
	if err := writeJSONLine(stdout, ready); err != nil {
		return exitOther
	}
	if f, ok := stdout.(interface{ Sync() error }); ok {
		_ = f.Sync()
	}
	logger.Info("listening", "addr", ln.Addr().String(), "version", version, "cache_dir", o.cacheDir)

	srv := &http.Server{
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       90 * time.Second,
		ErrorLog:          httpapi.ErrorLog(logger),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n, err := c.GC(); err == nil && n > 0 {
					logger.Info("cache gc", "removed", n)
				}
			}
		}
	}()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fail(o, logger, stdout, fmt.Errorf("http server: %w", err))
		}
		return exitOK
	case <-ctx.Done():
		logger.Info("shutdown requested")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Warn("graceful shutdown failed", "error", err.Error())
			_ = srv.Close()
		}
		return exitOK
	}
}

// ------------------------------------------------------------------- help text

func printHelp(w io.Writer) error {
	help := `feedmesh — multi-source content to RSS (single binary, no dependencies)

USAGE
  feedmesh <command> [flags]

COMMANDS
  serve                     Run the HTTP service
      --listen <addr>       Listen address (default 127.0.0.1:3000, env FEEDMESH_LISTEN; legacy WEIBO_RSS_PRO_LISTEN / WEIBO_RSS_LISTEN)
      --token <token>       Require "Authorization: Bearer <token>"; env FEEDMESH_TOKEN
      --rate-limit <n>      Per-IP requests per minute (0 disables; env FEEDMESH_RATE_LIMIT)
  fetch <uid|domain>        Print one account's timeline
      --json                Emit one line of JSON instead of RSS
      --limit <n>           Number of posts, 1-10 (default 10)
      --refresh             Bypass the cache and refetch from weibo
      -o, --output <path>   Write to a file; "-" means stdout (default)
  doctor                    Probe genvisitor, getIndex and statuses in order
      --uid <uid>           Account used for the probe (default 2803301701)
  cache stat                Report cache entry count and size
  cache purge --all         Delete every feedmesh cache entry
  cache purge --uid <uid>   Delete an account: profile, timeline, domain, feed and long-text entries
  cache purge --handle <h>  Delete an X timeline cached for @h
      --dry-run             Report what a purge would delete, delete nothing
  version                   Print the version
  help                      Print this text (same as -h), exit code 0

GLOBAL FLAGS
  --json                    Data output as exactly one line of JSON (jq-safe)
  --quiet, -q               Silence stderr logs
  --timeout <duration>      Per-upstream-request timeout; overrides all providers
                            (default 9s weibo, 15s sspai/V2EX/SMZDM/Reddit, 20s X/YouTube; X capped at 25s)
  --cache-dir <dir>         Cache directory (env FEEDMESH_CACHE; legacy WEIBO_RSS_PRO_CACHE / WEIBO_RSS_CACHE)
  -h, --help                Print this text

OUTPUT DISCIPLINE
  stdout: data only (RSS XML, or one line of JSON with --json)
  stderr: newline-delimited JSON logs ({"ts","level","msg",...})

EXIT CODES
  0   success
  2   usage error
  3   network failure or timeout
  4   upstream blocked (HTTP 418/403)
  5   user not found, or visible to logged-in users only
  6   circuit breaker open (cooling down after a block)
  10  anything else

EXAMPLES
  feedmesh fetch 2803301701
  feedmesh fetch 2803301701 --json --limit 5 | jq '.items[].text'
  feedmesh fetch rmrb --refresh -o rmrb.xml
  feedmesh doctor --json
  feedmesh cache purge --uid 2803301701 --dry-run --json
  feedmesh serve --listen 127.0.0.1:3000
`
	_, err := fmt.Fprint(w, help)
	return err
}
