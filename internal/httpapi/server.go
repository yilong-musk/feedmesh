package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yilong-musk/weibo-rss-pro/internal/cache"
	"github.com/yilong-musk/weibo-rss-pro/internal/feed"
	"github.com/yilong-musk/weibo-rss-pro/internal/guard"
	"github.com/yilong-musk/weibo-rss-pro/internal/logx"
	"github.com/yilong-musk/weibo-rss-pro/internal/weibo"
)

// probeFreshness is how long a /healthz?probe=1 result is reused.
const probeFreshness = 30 * time.Second

// Config wires the HTTP layer.
type Config struct {
	Version string
	Service *feed.Service
	Cache   *cache.Cache
	Client  *weibo.Client
	Breaker *guard.Breaker
	Log     *logx.Logger
	Now     func() time.Time
}

// Server holds the HTTP handlers.
type Server struct {
	cfg     Config
	started time.Time

	probeMu    sync.Mutex
	probeAt    time.Time
	probeSteps []weibo.Step
	probeOK    bool
}

// New builds a Server.
func New(cfg Config) *Server {
	if cfg.Log == nil {
		cfg.Log = logx.Discard()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Server{cfg: cfg, started: cfg.Now()}
}

// Handler returns the routed HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// Method-agnostic patterns so a wrong verb answers 405 with our error
	// envelope instead of the mux's plain-text default.
	mux.HandleFunc("/healthz", s.getOnly(s.handleHealth))
	mux.HandleFunc("/rss/user/{uid}", s.getOnly(s.handleUser))
	mux.HandleFunc("/rss/domain/{name}", s.getOnly(s.handleDomain))
	mux.HandleFunc("/api/v1/feed", s.getOnly(s.handleAPIFeed))
	mux.HandleFunc("/", s.handleNotFound)
	return mux
}

// getOnly rejects non-GET verbs with a 405 and the standard error envelope.
func (s *Server) getOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			s.writeErrorStatus(w, http.StatusMethodNotAllowed,
				weibo.NewError(weibo.CodeBadRequest, 0, "method %s is not allowed on %s", r.Method, r.URL.Path))
			return
		}
		h(w, r)
	}
}

type cacheInfo struct {
	Entries int   `json:"entries"`
	Bytes   int64 `json:"bytes"`
}

type breakerInfo struct {
	Open  bool       `json:"open"`
	Until *time.Time `json:"until"`
}

type healthResponse struct {
	Status  string       `json:"status"`
	Version string       `json:"version"`
	UptimeS int64        `json:"uptime_s"`
	Cache   cacheInfo    `json:"cache"`
	Breaker breakerInfo  `json:"breaker"`
	Probe   []weibo.Step `json:"probe,omitempty"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	out := healthResponse{
		Status:  "ok",
		Version: s.cfg.Version,
		UptimeS: int64(s.cfg.Now().Sub(s.started).Seconds()),
	}
	if s.cfg.Cache != nil {
		if st, err := s.cfg.Cache.Stat(); err == nil {
			out.Cache = cacheInfo{Entries: st.Entries, Bytes: st.Bytes}
		}
	}
	if s.cfg.Breaker != nil {
		state := s.cfg.Breaker.State()
		out.Breaker = breakerInfo{Open: state.Open, Until: state.Until}
	}
	if r.URL.Query().Get("probe") == "1" {
		steps, _ := s.probe(r.Context())
		out.Probe = steps
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) probe(ctx context.Context) ([]weibo.Step, bool) {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	if len(s.probeSteps) > 0 && s.cfg.Now().Sub(s.probeAt) < probeFreshness {
		return s.probeSteps, s.probeOK
	}
	steps := s.cfg.Client.Probe(ctx, "")
	ok := true
	for _, st := range steps {
		if !st.OK {
			ok = false
			break
		}
	}
	s.probeSteps, s.probeAt, s.probeOK = steps, s.cfg.Now(), ok
	return steps, ok
}

func (s *Server) handleUser(w http.ResponseWriter, r *http.Request) {
	uid := strings.TrimSuffix(r.PathValue("uid"), ".xml")
	s.serveRSS(w, r, uid)
}

func (s *Server) handleDomain(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSuffix(r.PathValue("name"), ".xml")
	uid, err := s.cfg.Service.ResolveUID(r.Context(), name, false)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.serveRSS(w, r, uid)
}

func (s *Server) serveRSS(w http.ResponseWriter, r *http.Request, uid string) {
	limit := atoiDefault(r.URL.Query().Get("limit"), feed.DefaultLimit)
	refresh := r.URL.Query().Get("refresh") == "1"

	f, err := s.cfg.Service.GetFeed(r.Context(), uid, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(feed.RenderRSS(f))
}

type feedResponse struct {
	UID         string      `json:"uid"`
	ScreenName  string      `json:"screen_name"`
	Description string      `json:"description"`
	FetchedAt   time.Time   `json:"fetched_at"`
	Cached      bool        `json:"cached"`
	Items       []feed.Item `json:"items"`
}

func (s *Server) handleAPIFeed(w http.ResponseWriter, r *http.Request) {
	uid := strings.TrimSpace(r.URL.Query().Get("uid"))
	if uid == "" {
		s.writeError(w, weibo.NewError(weibo.CodeBadRequest, 0, "query parameter uid is required"))
		return
	}
	limit := atoiDefault(r.URL.Query().Get("limit"), feed.DefaultLimit)

	f, err := s.cfg.Service.GetFeed(r.Context(), uid, limit, false)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, feedResponse{
		UID:         f.UID,
		ScreenName:  f.ScreenName,
		Description: f.Description,
		FetchedAt:   f.FetchedAt,
		Cached:      f.Cached,
		Items:       f.Items,
	})
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	s.writeError(w, weibo.NewError(weibo.CodeBadRequest, 0,
		"unknown endpoint %s %s", r.Method, r.URL.Path))
}

func atoiDefault(raw string, def int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	if n > feed.MaxLimit {
		return feed.MaxLimit
	}
	return n
}
