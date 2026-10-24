package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/feed"
	"github.com/yilong-musk/feedmesh/internal/guard"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/weibo"
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

	// Token enables bearer-token authentication when non-empty. RateLimit is
	// the per-IP request budget per minute (zero disables it). RequestTimeout
	// bounds one HTTP handler; zero uses the default.
	Token          string
	RateLimit      int
	RequestTimeout time.Duration
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
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 3 * time.Minute
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
	mux.HandleFunc("/rss/hot", s.getOnly(s.handleHot))
	mux.HandleFunc("/rss/hot.xml", s.getOnly(s.handleHot))
	mux.HandleFunc("/rss/sspai", s.getOnly(s.handleSspai))
	mux.HandleFunc("/rss/sspai.xml", s.getOnly(s.handleSspai))
	mux.HandleFunc("/rss/v2ex", s.getOnly(s.handleV2exLatest))
	mux.HandleFunc("/rss/v2ex.xml", s.getOnly(s.handleV2exLatest))
	mux.HandleFunc("/rss/v2ex/hot", s.getOnly(s.handleV2exHot))
	mux.HandleFunc("/rss/v2ex/hot.xml", s.getOnly(s.handleV2exHot))
	mux.HandleFunc("/rss/x/{handle}", s.getOnly(s.handleX))
	mux.HandleFunc("/api/v1/feed", s.getOnly(s.handleAPIFeed))
	mux.HandleFunc("/api/v1/hot", s.getOnly(s.handleAPIHot))
	mux.HandleFunc("/api/v1/sspai", s.getOnly(s.handleAPISspai))
	mux.HandleFunc("/api/v1/v2ex", s.getOnly(s.handleAPIV2ex))
	mux.HandleFunc("/api/v1/x", s.getOnly(s.handleAPIX))
	mux.HandleFunc("/", s.handleNotFound)

	var h http.Handler = mux
	h = s.withTimeout(h)
	if s.cfg.Token != "" {
		h = s.withAuth(h)
	}
	if s.cfg.RateLimit > 0 {
		h = s.withRateLimit(h)
	}
	h = s.withRecovery(h)
	return h
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
	w.Header().Set("Cache-Control", "no-store")
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
	refresh := r.URL.Query().Get("refresh") == "1"
	uid, err := s.cfg.Service.ResolveUID(r.Context(), name, refresh)
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
	refresh := r.URL.Query().Get("refresh") == "1"

	f, err := s.cfg.Service.GetFeed(r.Context(), uid, limit, refresh)
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

type hotItemResponse struct {
	Rank      int       `json:"rank"`
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	URL       string    `json:"url"`
	HTML      string    `json:"html"`
	CreatedAt time.Time `json:"created_at"`
}

type hotResponse struct {
	Title       string            `json:"title"`
	Description string            `json:"description"`
	FetchedAt   time.Time         `json:"fetched_at"`
	Cached      bool              `json:"cached"`
	Items       []hotItemResponse `json:"items"`
}

func (s *Server) handleHot(w http.ResponseWriter, r *http.Request) {
	limit := atoiClamp(r.URL.Query().Get("limit"), feed.DefaultHotLimit, feed.MaxHotLimit)
	refresh := r.URL.Query().Get("refresh") == "1"

	f, err := s.cfg.Service.GetHot(r.Context(), limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(feed.RenderRSS(f))
}

func (s *Server) handleAPIHot(w http.ResponseWriter, r *http.Request) {
	limit := atoiClamp(r.URL.Query().Get("limit"), feed.DefaultHotLimit, feed.MaxHotLimit)

	f, err := s.cfg.Service.GetHot(r.Context(), limit, false)
	if err != nil {
		s.writeError(w, err)
		return
	}
	items := make([]hotItemResponse, 0, len(f.Items))
	for i, it := range f.Items {
		items = append(items, hotItemResponse{
			Rank: i + 1, Title: it.Title, Text: it.Text,
			URL: it.URL, HTML: it.HTML, CreatedAt: it.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, hotResponse{
		Title:       f.Title(),
		Description: f.Description,
		FetchedAt:   f.FetchedAt,
		Cached:      f.Cached,
		Items:       items,
	})
}

type sspaiItemResponse struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	URL       string    `json:"url"`
	HTML      string    `json:"html"`
	Images    []string  `json:"images"`
	CreatedAt time.Time `json:"created_at"`
}

type sspaiResponse struct {
	Title       string              `json:"title"`
	Description string              `json:"description"`
	FetchedAt   time.Time           `json:"fetched_at"`
	Cached      bool                `json:"cached"`
	Items       []sspaiItemResponse `json:"items"`
}

func (s *Server) handleSspai(w http.ResponseWriter, r *http.Request) {
	limit := atoiClamp(r.URL.Query().Get("limit"), feed.DefaultSspaiLimit, feed.MaxSspaiLimit)
	full := r.URL.Query().Get("full") == "1"
	refresh := r.URL.Query().Get("refresh") == "1"

	f, err := s.cfg.Service.GetSspai(r.Context(), limit, full, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(feed.RenderRSS(f))
}

func (s *Server) handleAPISspai(w http.ResponseWriter, r *http.Request) {
	limit := atoiClamp(r.URL.Query().Get("limit"), feed.DefaultSspaiLimit, feed.MaxSspaiLimit)
	full := r.URL.Query().Get("full") == "1"
	refresh := r.URL.Query().Get("refresh") == "1"

	f, err := s.cfg.Service.GetSspai(r.Context(), limit, full, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	items := make([]sspaiItemResponse, 0, len(f.Items))
	for _, it := range f.Items {
		items = append(items, sspaiItemResponse{
			ID: it.ID, Title: it.Title, Text: it.Text, URL: it.URL,
			HTML: it.HTML, Images: it.Images, CreatedAt: it.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, sspaiResponse{
		Title: f.Title(), Description: f.Description,
		FetchedAt: f.FetchedAt, Cached: f.Cached, Items: items,
	})
}

type v2exItemResponse struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	URL       string    `json:"url"`
	HTML      string    `json:"html"`
	Images    []string  `json:"images"`
	CreatedAt time.Time `json:"created_at"`
}

type v2exResponse struct {
	Title       string             `json:"title"`
	Description string             `json:"description"`
	FetchedAt   time.Time          `json:"fetched_at"`
	Cached      bool               `json:"cached"`
	Items       []v2exItemResponse `json:"items"`
}

func (s *Server) serveV2ex(w http.ResponseWriter, r *http.Request, section string) {
	limit := atoiClamp(r.URL.Query().Get("limit"), feed.DefaultV2exLimit, feed.MaxV2exLimit)
	refresh := r.URL.Query().Get("refresh") == "1"
	f, err := s.cfg.Service.GetV2ex(r.Context(), section, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(feed.RenderRSS(f))
}

func (s *Server) handleV2exLatest(w http.ResponseWriter, r *http.Request) {
	s.serveV2ex(w, r, "latest")
}
func (s *Server) handleV2exHot(w http.ResponseWriter, r *http.Request) { s.serveV2ex(w, r, "hot") }

func (s *Server) handleAPIV2ex(w http.ResponseWriter, r *http.Request) {
	section := r.URL.Query().Get("section")
	if section == "" {
		section = "latest"
	}
	limit := atoiClamp(r.URL.Query().Get("limit"), feed.DefaultV2exLimit, feed.MaxV2exLimit)
	refresh := r.URL.Query().Get("refresh") == "1"
	f, err := s.cfg.Service.GetV2ex(r.Context(), section, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	items := make([]v2exItemResponse, 0, len(f.Items))
	for _, it := range f.Items {
		items = append(items, v2exItemResponse{
			ID: it.ID, Title: it.Title, Text: it.Text, URL: it.URL,
			HTML: it.HTML, Images: it.Images, CreatedAt: it.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, v2exResponse{
		Title: f.Title(), Description: f.Description,
		FetchedAt: f.FetchedAt, Cached: f.Cached, Items: items,
	})
}

type xItemResponse struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	URL       string    `json:"url"`
	HTML      string    `json:"html"`
	Images    []string  `json:"images"`
	CreatedAt time.Time `json:"created_at"`
}

type xResponse struct {
	Handle      string          `json:"handle"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	FetchedAt   time.Time       `json:"fetched_at"`
	Cached      bool            `json:"cached"`
	Items       []xItemResponse `json:"items"`
}

func (s *Server) handleX(w http.ResponseWriter, r *http.Request) {
	limit := atoiClamp(r.URL.Query().Get("limit"), feed.DefaultXLimit, feed.MaxXLimit)
	refresh := r.URL.Query().Get("refresh") == "1"
	handle := strings.TrimSuffix(r.PathValue("handle"), ".xml")
	f, err := s.cfg.Service.GetX(r.Context(), handle, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	// Nitter's own <ttl> is 40 minutes; match the cache rather than inviting
	// readers to hammer volunteer instances.
	w.Header().Set("Cache-Control", "public, max-age=2400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(feed.RenderRSS(f))
}

func (s *Server) handleAPIX(w http.ResponseWriter, r *http.Request) {
	handle := r.URL.Query().Get("handle")
	limit := atoiClamp(r.URL.Query().Get("limit"), feed.DefaultXLimit, feed.MaxXLimit)
	refresh := r.URL.Query().Get("refresh") == "1"
	f, err := s.cfg.Service.GetX(r.Context(), handle, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	items := make([]xItemResponse, 0, len(f.Items))
	for _, it := range f.Items {
		items = append(items, xItemResponse{
			ID: it.ID, Title: it.Title, Text: it.Text, URL: it.URL,
			HTML: it.HTML, Images: it.Images, CreatedAt: it.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, xResponse{
		Handle: f.UID, Title: f.Title(), Description: f.Description,
		FetchedAt: f.FetchedAt, Cached: f.Cached, Items: items,
	})
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	s.writeError(w, weibo.NewError(weibo.CodeBadRequest, 0,
		"unknown endpoint %s %s", r.Method, r.URL.Path))
}

func atoiDefault(raw string, def int) int {
	return atoiClamp(raw, def, feed.MaxLimit)
}

// atoiClamp parses a positive integer, falling back to def and capping at max.
func atoiClamp(raw string, def, max int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}
