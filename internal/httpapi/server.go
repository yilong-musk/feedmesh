package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yilong-musk/feedmesh/internal/apperr"
	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/feed"
	"github.com/yilong-musk/feedmesh/internal/guard"
	"github.com/yilong-musk/feedmesh/internal/linuxdo"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/reddit"
	"github.com/yilong-musk/feedmesh/internal/smzdm"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

// probeFreshness is how long a /healthz?probe=1 result is reused.
const probeFreshness = 30 * time.Second

// degradedRSSMaxAge is the reader-facing lifetime of any other document served
// from its last good cache entry: short enough that readers pick the real feed
// up soon after the upstream recovers, even though the content may be hours old.
const degradedRSSMaxAge = 60

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
	mux.HandleFunc("/rss/smzdm", s.getOnly(s.handleSmzdm))
	mux.HandleFunc("/rss/smzdm.xml", s.getOnly(s.handleSmzdm))
	mux.HandleFunc("/rss/smzdm/post", s.getOnly(s.handleSmzdmPost))
	mux.HandleFunc("/rss/smzdm/post.xml", s.getOnly(s.handleSmzdmPost))
	mux.HandleFunc("/rss/smzdm/haitao", s.getOnly(s.handleSmzdmHaitao))
	mux.HandleFunc("/rss/smzdm/haitao.xml", s.getOnly(s.handleSmzdmHaitao))
	mux.HandleFunc("/rss/smzdm/fenlei/{slug}", s.getOnly(s.handleSmzdmFenlei))
	mux.HandleFunc("/rss/x/{handle}", s.getOnly(s.handleX))
	mux.HandleFunc("/rss/youtube/{ref}", s.getOnly(s.handleYouTube))
	mux.HandleFunc("/rss/douyin/{sec}", s.getOnly(s.handleDouyin))
	mux.HandleFunc("/rss/tiktok/{user}", s.getOnly(s.handleTikTok))
	mux.HandleFunc("/rss/bilibili/{mid}", s.getOnly(s.handleBilibili))
	mux.HandleFunc("/rss/telegram/{channel}", s.getOnly(s.handleTelegram))
	mux.HandleFunc("/rss/pinterest/{user}", s.getOnly(s.handlePinterest))
	mux.HandleFunc("/rss/pinterest/{user}/{board}", s.getOnly(s.handlePinterest))
	mux.HandleFunc("/rss/polymarket/new", s.getOnly(s.handlePolymarketNew))
	mux.HandleFunc("/rss/polymarket/new.xml", s.getOnly(s.handlePolymarketNew))
	mux.HandleFunc("/rss/polymarket/hot", s.getOnly(s.handlePolymarketHot))
	mux.HandleFunc("/rss/polymarket/hot.xml", s.getOnly(s.handlePolymarketHot))
	mux.HandleFunc("/rss/polymarket/event/{slug}", s.getOnly(s.handlePolymarketEvent))
	mux.HandleFunc("/rss/polymarket/user/{address}", s.getOnly(s.handlePolymarketUser))
	mux.HandleFunc("/rss/linuxdo", s.getOnly(s.handleLinuxDoLatest))
	mux.HandleFunc("/rss/linuxdo.xml", s.getOnly(s.handleLinuxDoLatest))
	mux.HandleFunc("/rss/linuxdo/top", s.getOnly(s.handleLinuxDoTop))
	mux.HandleFunc("/rss/linuxdo/top.xml", s.getOnly(s.handleLinuxDoTop))
	mux.HandleFunc("/rss/linuxdo/hot", s.getOnly(s.handleLinuxDoHot))
	mux.HandleFunc("/rss/linuxdo/hot.xml", s.getOnly(s.handleLinuxDoHot))
	mux.HandleFunc("/rss/linuxdo/c/{slug}/{id}", s.getOnly(s.handleLinuxDoCategory))
	mux.HandleFunc("/rss/linuxdo/c/{id}", s.getOnly(s.handleLinuxDoCategory))
	mux.HandleFunc("/rss/linuxdo/tag/{tag}", s.getOnly(s.handleLinuxDoTag))
	mux.HandleFunc("/rss/reddit/r/{sub}", s.getOnly(s.handleReddit))
	mux.HandleFunc("/rss/reddit/{sub}", s.getOnly(s.handleReddit))
	mux.HandleFunc("/api/v1/feed", s.getOnly(s.handleAPIFeed))
	mux.HandleFunc("/api/v1/hot", s.getOnly(s.handleAPIHot))
	mux.HandleFunc("/api/v1/sspai", s.getOnly(s.handleAPISspai))
	mux.HandleFunc("/api/v1/v2ex", s.getOnly(s.handleAPIV2ex))
	mux.HandleFunc("/api/v1/smzdm", s.getOnly(s.handleAPISmzdm))
	mux.HandleFunc("/api/v1/x", s.getOnly(s.handleAPIX))
	mux.HandleFunc("/api/v1/youtube", s.getOnly(s.handleAPIYouTube))
	mux.HandleFunc("/api/v1/douyin", s.getOnly(s.handleAPIDouyin))
	mux.HandleFunc("/api/v1/douyin/timeline", s.getOnly(s.handleAPIDouyinTimeline))
	mux.HandleFunc("/api/v1/tiktok", s.getOnly(s.handleAPITikTok))
	mux.HandleFunc("/api/v1/tiktok/timeline", s.getOnly(s.handleAPITikTokTimeline))
	mux.HandleFunc("/api/v1/bilibili", s.getOnly(s.handleAPIBilibili))
	mux.HandleFunc("/api/v1/telegram", s.getOnly(s.handleAPITelegram))
	mux.HandleFunc("/api/v1/pinterest", s.getOnly(s.handleAPIPinterest))
	mux.HandleFunc("/api/v1/polymarket", s.getOnly(s.handleAPIPolymarket))
	mux.HandleFunc("/api/v1/linuxdo", s.getOnly(s.handleAPILinuxDo))
	mux.HandleFunc("/api/v1/reddit", s.getOnly(s.handleAPIReddit))
	mux.HandleFunc("/", s.handleNotFound)

	var h http.Handler = mux
	h = s.withGzip(h)
	h = s.withTimeout(h)
	if s.cfg.Token != "" {
		h = s.withAuth(h)
	}
	if s.cfg.RateLimit > 0 {
		h = s.withRateLimit(h)
	}
	h = s.withRecovery(h)
	h = s.withErrorLog(h)
	return h
}

// setRSSCache emits cache directives appropriate for the response. When a
// bearer token is configured, a shared cache must not reuse an authenticated
// response for an unauthenticated client.
func (s *Server) setRSSCache(w http.ResponseWriter, maxAge int) {
	if s.cfg.Token != "" {
		w.Header().Set("Cache-Control", "private, max-age="+strconv.Itoa(maxAge))
		w.Header().Add("Vary", "Authorization")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(maxAge))
}

// setRSSCacheFor emits the cache directives for a rendered feed and marks a
// degraded document: the header says why the content can be old, and the short
// max-age lets the reader pick up the fresh feed soon after recovery.
func (s *Server) setRSSCacheFor(w http.ResponseWriter, f *feed.Feed, maxAge int) {
	if f.Degraded {
		w.Header().Set("X-Feedmesh-Degraded", "1")
		maxAge = degradedRSSMaxAge
	}
	s.setRSSCache(w, maxAge)
}

// getOnly rejects non-GET verbs with a 405 and the standard error envelope.
func (s *Server) getOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			s.writeErrorStatus(w, http.StatusMethodNotAllowed,
				apperr.New(apperr.CodeBadRequest, 0, "method %s is not allowed on %s", r.Method, r.URL.Path))
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
	refresh := refreshParam(r)
	uid, err := s.cfg.Service.ResolveUID(r.Context(), name, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.serveRSS(w, r, uid)
}

func (s *Server) serveRSS(w http.ResponseWriter, r *http.Request, uid string) {
	limit := atoiDefault(r.URL.Query().Get("limit"), feed.DefaultLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetFeed(r.Context(), uid, limit, refresh)
	s.serveFeed(w, r, f, err, 300)
}

type feedResponse struct {
	UID         string      `json:"uid"`
	ScreenName  string      `json:"screen_name"`
	Description string      `json:"description"`
	FetchedAt   time.Time   `json:"fetched_at"`
	Cached      bool        `json:"cached"`
	Degraded    bool        `json:"degraded"`
	Items       []feed.Item `json:"items"`
}

func (s *Server) handleAPIFeed(w http.ResponseWriter, r *http.Request) {
	uid := strings.TrimSpace(r.URL.Query().Get("uid"))
	if uid == "" {
		s.writeError(w, apperr.New(apperr.CodeBadRequest, 0, "query parameter uid is required"))
		return
	}
	limit := atoiDefault(r.URL.Query().Get("limit"), feed.DefaultLimit)
	refresh := refreshParam(r)

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
		Degraded:    f.Degraded,
		Items:       f.Items,
	})
}

type hotItemResponse struct {
	Rank      int       `json:"rank"`
	Heat      int64     `json:"heat"`
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
	Degraded    bool              `json:"degraded"`
	Items       []hotItemResponse `json:"items"`
}

func (s *Server) handleHot(w http.ResponseWriter, r *http.Request) {
	limit := limitParam(r, feed.DefaultHotLimit, feed.MaxHotLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetHot(r.Context(), limit, refresh)
	s.serveFeed(w, r, f, err, 60)
}

func (s *Server) handleAPIHot(w http.ResponseWriter, r *http.Request) {
	limit := limitParam(r, feed.DefaultHotLimit, feed.MaxHotLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetHot(r.Context(), limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	items := make([]hotItemResponse, 0, len(f.Items))
	for i, it := range f.Items {
		rank := it.Rank
		if rank == 0 {
			rank = i + 1
		}
		items = append(items, hotItemResponse{
			Rank: rank, Heat: it.Heat, Title: it.Title, Text: it.Text,
			URL: it.URL, HTML: it.HTML, CreatedAt: it.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, hotResponse{
		Title:       f.Title(),
		Description: f.Description,
		FetchedAt:   f.FetchedAt,
		Cached:      f.Cached,
		Degraded:    f.Degraded,
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
	Degraded    bool                `json:"degraded"`
	Items       []sspaiItemResponse `json:"items"`
}

func (s *Server) handleSspai(w http.ResponseWriter, r *http.Request) {
	limit := limitParam(r, feed.DefaultSspaiLimit, feed.MaxSspaiLimit)
	full := r.URL.Query().Get("full") == "1"
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetSspai(r.Context(), limit, full, refresh)
	s.serveFeed(w, r, f, err, 600)
}

func (s *Server) handleAPISspai(w http.ResponseWriter, r *http.Request) {
	limit := limitParam(r, feed.DefaultSspaiLimit, feed.MaxSspaiLimit)
	full := r.URL.Query().Get("full") == "1"
	refresh := refreshParam(r)

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
		FetchedAt: f.FetchedAt, Cached: f.Cached, Degraded: f.Degraded, Items: items,
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
	Degraded    bool               `json:"degraded"`
	Items       []v2exItemResponse `json:"items"`
}

func (s *Server) serveV2ex(w http.ResponseWriter, r *http.Request, section string) {
	limit := limitParam(r, feed.DefaultV2exLimit, feed.MaxV2exLimit)
	refresh := refreshParam(r)
	f, err := s.cfg.Service.GetV2ex(r.Context(), section, limit, refresh)
	s.serveFeed(w, r, f, err, 600)
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
	limit := limitParam(r, feed.DefaultV2exLimit, feed.MaxV2exLimit)
	refresh := refreshParam(r)
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
		FetchedAt: f.FetchedAt, Cached: f.Cached, Degraded: f.Degraded, Items: items,
	})
}

type smzdmItemResponse struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	URL       string    `json:"url"`
	HTML      string    `json:"html"`
	Images    []string  `json:"images"`
	CreatedAt time.Time `json:"created_at"`
	// Price is the listed amount of a good-price card. Category pages publish
	// one; the RSS channels do not, so the field is null there.
	Price *float64 `json:"price"`
	// Slug names the category a card came from (category channel only).
	Slug string `json:"slug,omitempty"`
	// DateSource says how created_at was derived (category channel only):
	// "fetched" when the page read time stands in for a missing publish
	// timestamp, "card" when the card carried its own.
	DateSource string `json:"date_source,omitempty"`
}

type smzdmResponse struct {
	Channel     string              `json:"channel"`
	Slug        string              `json:"slug,omitempty"`
	Title       string              `json:"title"`
	Description string              `json:"description"`
	FetchedAt   time.Time           `json:"fetched_at"`
	Cached      bool                `json:"cached"`
	Degraded    bool                `json:"degraded"`
	Items       []smzdmItemResponse `json:"items"`
}

// smzdmItems maps the feed model onto the SMZDM JSON view.
func smzdmItems(items []feed.Item) []smzdmItemResponse {
	out := make([]smzdmItemResponse, 0, len(items))
	for _, it := range items {
		out = append(out, smzdmItemResponse{
			ID: it.ID, Title: it.Title, Author: it.Author, Text: it.Text, URL: it.URL,
			HTML: it.HTML, Images: it.Images, CreatedAt: it.CreatedAt,
			Price: it.Price, Slug: it.Slug, DateSource: it.DateSource,
		})
	}
	return out
}

// serveSmzdm renders one channel as RSS.
func (s *Server) serveSmzdm(w http.ResponseWriter, r *http.Request, ch smzdm.Channel) {
	limit := limitParam(r, feed.DefaultSmzdmLimit, feed.MaxSmzdmLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetSmzdm(r.Context(), ch, limit, refresh)
	s.serveFeed(w, r, f, err, int(feed.SmzdmTTL(ch).Seconds()))
}

func (s *Server) handleSmzdm(w http.ResponseWriter, r *http.Request) {
	s.serveSmzdm(w, r, smzdm.ChannelDeals)
}

func (s *Server) handleSmzdmPost(w http.ResponseWriter, r *http.Request) {
	s.serveSmzdm(w, r, smzdm.ChannelPost)
}

func (s *Server) handleSmzdmHaitao(w http.ResponseWriter, r *http.Request) {
	s.serveSmzdm(w, r, smzdm.ChannelHaitao)
}

// handleSmzdmFenlei renders one category page as RSS. The feed is cached for
// an hour upstream-side, so the reader-facing max-age matches it: a reader that
// reloads sooner gets the same document anyway.
func (s *Server) handleSmzdmFenlei(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSuffix(r.PathValue("slug"), ".xml")
	limit := limitParam(r, feed.DefaultSmzdmLimit, feed.MaxSmzdmLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetSmzdmFenlei(r.Context(), slug, limit, refresh)
	s.serveFeed(w, r, f, err, int(feed.SmzdmFenleiTTL().Seconds()))
}

func (s *Server) handleAPISmzdm(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("channel")
	ch, ok := smzdm.ParseChannel(raw)
	if !ok {
		s.writeError(w, &apperr.Error{
			Code:    apperr.CodeBadRequest,
			Message: "unknown smzdm channel " + strconv.Quote(raw),
		})
		return
	}
	limit := limitParam(r, feed.DefaultSmzdmLimit, feed.MaxSmzdmLimit)
	refresh := refreshParam(r)

	// The category channel needs a slug; the RSS channels ignore it.
	if ch == smzdm.ChannelFenlei {
		slug := strings.TrimSpace(r.URL.Query().Get("slug"))
		if slug == "" {
			s.writeError(w, &apperr.Error{
				Code:    apperr.CodeBadRequest,
				Message: "query parameter slug is required for channel fenlei",
			})
			return
		}
		f, err := s.cfg.Service.GetSmzdmFenlei(r.Context(), slug, limit, refresh)
		if err != nil {
			s.writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, smzdmResponse{
			Channel: string(ch), Slug: strings.ToLower(slug), Title: f.Title(),
			Description: f.Description, FetchedAt: f.FetchedAt, Cached: f.Cached, Degraded: f.Degraded,
			Items: smzdmItems(f.Items),
		})
		return
	}

	f, err := s.cfg.Service.GetSmzdm(r.Context(), ch, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, smzdmResponse{
		Channel: string(ch), Title: f.Title(), Description: f.Description,
		FetchedAt: f.FetchedAt, Cached: f.Cached, Degraded: f.Degraded, Items: smzdmItems(f.Items),
	})
}

type xVideoResponse struct {
	URL        string `json:"url,omitempty"`
	Thumbnail  string `json:"thumbnail,omitempty"`
	DurationMS int    `json:"duration_ms,omitempty"`
	Views      int64  `json:"views,omitempty"`
}

type xQuotedResponse struct {
	ID         string          `json:"id,omitempty"`
	URL        string          `json:"url,omitempty"`
	Author     string          `json:"author,omitempty"`
	AuthorName string          `json:"author_name,omitempty"`
	Text       string          `json:"text,omitempty"`
	Images     []string        `json:"images,omitempty"`
	Video      *xVideoResponse `json:"video,omitempty"`
}

type xItemResponse struct {
	ID        string           `json:"id"`
	Title     string           `json:"title"`
	Text      string           `json:"text"`
	URL       string           `json:"url"`
	HTML      string           `json:"html"`
	Author    string           `json:"author,omitempty"`
	AuthorID  string           `json:"author_id,omitempty"`
	Images    []string         `json:"images"`
	CardImage string           `json:"card_image,omitempty"`
	Video     *xVideoResponse  `json:"video,omitempty"`
	Quoted    *xQuotedResponse `json:"quoted,omitempty"`
	Likes     int              `json:"likes"`
	Replies   int              `json:"replies"`
	CreatedAt time.Time        `json:"created_at"`
}

type xResponse struct {
	Handle      string          `json:"handle"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	FetchedAt   time.Time       `json:"fetched_at"`
	Cached      bool            `json:"cached"`
	Degraded    bool            `json:"degraded"`
	Items       []xItemResponse `json:"items"`
}

func (s *Server) handleX(w http.ResponseWriter, r *http.Request) {
	limit := limitParam(r, feed.DefaultXLimit, feed.MaxXLimit)
	refresh := refreshParam(r)
	handle := strings.TrimSuffix(r.PathValue("handle"), ".xml")
	f, err := s.cfg.Service.GetX(r.Context(), handle, limit, refresh)
	// Nitter's own <ttl> is 40 minutes; match the cache rather than inviting
	// readers to hammer volunteer instances.
	s.serveFeed(w, r, f, err, 2400)
}

func (s *Server) handleAPIX(w http.ResponseWriter, r *http.Request) {
	handle := r.URL.Query().Get("handle")
	limit := limitParam(r, feed.DefaultXLimit, feed.MaxXLimit)
	refresh := refreshParam(r)
	f, err := s.cfg.Service.GetX(r.Context(), handle, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	items := make([]xItemResponse, 0, len(f.Items))
	for _, it := range f.Items {
		item := xItemResponse{
			ID: it.ID, Title: it.Title, Text: it.Text, URL: it.URL,
			HTML: it.HTML, Images: it.Images, Author: it.Author,
			AuthorID: it.AuthorID, CardImage: it.CardImage,
			Likes: it.Likes, Replies: it.Replies, CreatedAt: it.CreatedAt,
		}
		if it.VideoURL != "" || it.PosterURL != "" {
			item.Video = &xVideoResponse{
				URL:        it.VideoURL,
				Thumbnail:  it.PosterURL,
				DurationMS: it.VideoDurationMS,
				Views:      it.VideoViews,
			}
		}
		if q := it.Quoted; q != nil {
			quoted := &xQuotedResponse{
				ID: q.ID, URL: q.URL, Author: q.Author, AuthorName: q.AuthorName,
				Text: q.Text, Images: q.Images,
			}
			if q.VideoURL != "" || q.VideoPoster != "" {
				quoted.Video = &xVideoResponse{
					URL: q.VideoURL, Thumbnail: q.VideoPoster,
					DurationMS: q.DurationMS, Views: q.Views,
				}
			}
			item.Quoted = quoted
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, xResponse{
		Handle: f.UID, Title: f.Title(), Description: f.Description,
		FetchedAt: f.FetchedAt, Cached: f.Cached, Degraded: f.Degraded, Items: items,
	})
}

type youtubeItemResponse struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	URL       string    `json:"url"`
	HTML      string    `json:"html"`
	Images    []string  `json:"images"`
	Views     int64     `json:"views"`
	CreatedAt time.Time `json:"created_at"`
}

type youtubeResponse struct {
	Ref         string                `json:"ref"`
	ChannelID   string                `json:"channel_id"`
	Title       string                `json:"title"`
	Description string                `json:"description"`
	FetchedAt   time.Time             `json:"fetched_at"`
	Cached      bool                  `json:"cached"`
	Degraded    bool                  `json:"degraded"`
	Source      string                `json:"source"`
	Items       []youtubeItemResponse `json:"items"`
}

// setSource publishes where a list came from: atom (the official document) or
// stale (the last good cached list). It is the only signal an operator gets
// that an outage window or a rate-limit window is in progress.
func setSource(w http.ResponseWriter, source string) {
	if source != "" {
		w.Header().Set("X-Feedmesh-Source", source)
	}
}

// handleYouTube renders one channel's upload feed as RSS.
func (s *Server) handleYouTube(w http.ResponseWriter, r *http.Request) {
	ref := strings.TrimSuffix(r.PathValue("ref"), ".xml")
	limit := limitParam(r, feed.DefaultYouTubeLimit, feed.MaxYouTubeLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetYouTube(r.Context(), ref, limit, refresh)
	s.serveFeed(w, r, f, err, int(feed.YouTubeTTL().Seconds()))
}

func (s *Server) handleAPIYouTube(w http.ResponseWriter, r *http.Request) {
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	limit := limitParam(r, feed.DefaultYouTubeLimit, feed.MaxYouTubeLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetYouTube(r.Context(), ref, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	items := make([]youtubeItemResponse, 0, len(f.Items))
	for _, it := range f.Items {
		items = append(items, youtubeItemResponse{
			ID: it.ID, Title: it.Title, Text: it.Text, URL: it.URL,
			HTML: it.HTML, Images: it.Images, Views: it.Views, CreatedAt: it.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, youtubeResponse{
		Ref: ref, ChannelID: f.UID, Title: f.Title(), Description: f.Description,
		FetchedAt: f.FetchedAt, Cached: f.Cached, Degraded: f.Degraded,
		Source: f.Source, Items: items,
	})
}

type redditItemResponse struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	URL       string    `json:"url"`
	HTML      string    `json:"html"`
	Images    []string  `json:"images"`
	CreatedAt time.Time `json:"created_at"`
}

type redditResponse struct {
	Sub         string               `json:"sub"`
	Sort        string               `json:"sort"`
	Window      string               `json:"window"`
	Title       string               `json:"title"`
	Description string               `json:"description"`
	FetchedAt   time.Time            `json:"fetched_at"`
	Cached      bool                 `json:"cached"`
	Degraded    bool                 `json:"degraded"`
	Source      string               `json:"source"`
	Items       []redditItemResponse `json:"items"`
}

// redditListing reads the listing a request names. An unknown sort or window is
// a client error, not an upstream failure.
func redditListing(r *http.Request) (reddit.Sort, reddit.Window, error) {
	rawSort := r.URL.Query().Get("sort")
	sort, ok := reddit.ParseSort(rawSort)
	if !ok {
		return "", "", &apperr.Error{
			Code:    apperr.CodeBadRequest,
			Message: "unknown sort " + strconv.Quote(rawSort) + ": use hot, new or top",
		}
	}
	rawWindow := r.URL.Query().Get("window")
	window, ok := reddit.ParseWindow(rawWindow)
	if !ok {
		return "", "", &apperr.Error{
			Code:    apperr.CodeBadRequest,
			Message: "unknown window " + strconv.Quote(rawWindow) + ": use day, week, month, year or all",
		}
	}
	return sort, window, nil
}

// handleReddit renders one subreddit listing as RSS. The path is
// /rss/reddit/{sub}; the /rss/reddit/r/{sub} form accepts the name with the
// "r/" prefix a reader copies from the site.
func (s *Server) handleReddit(w http.ResponseWriter, r *http.Request) {
	sub := strings.TrimSuffix(r.PathValue("sub"), ".xml")
	sort, window, err := redditListing(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	limit := limitParam(r, feed.DefaultRedditLimit, feed.MaxRedditLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetReddit(r.Context(), sub, sort, window, limit, refresh)
	s.serveFeed(w, r, f, err, int(feed.RedditTTL().Seconds()))
}

func (s *Server) handleAPIReddit(w http.ResponseWriter, r *http.Request) {
	sub := strings.TrimSpace(r.URL.Query().Get("sub"))
	if sub == "" {
		s.writeError(w, apperr.New(apperr.CodeBadRequest, 0, "query parameter sub is required"))
		return
	}
	sort, window, err := redditListing(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	limit := limitParam(r, feed.DefaultRedditLimit, feed.MaxRedditLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetReddit(r.Context(), sub, sort, window, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	items := make([]redditItemResponse, 0, len(f.Items))
	for _, it := range f.Items {
		items = append(items, redditItemResponse{
			ID: it.ID, Title: it.Title, Author: it.Author, Text: it.Text, URL: it.URL,
			HTML: it.HTML, Images: it.Images, CreatedAt: it.CreatedAt,
		})
	}
	if f.Degraded {
		w.Header().Set("X-Feedmesh-Degraded", "1")
	}
	setSource(w, f.Source)
	writeJSON(w, http.StatusOK, redditResponse{
		Sub: f.UID, Sort: string(sort), Window: string(window), Title: f.Title(),
		Description: f.Description, FetchedAt: f.FetchedAt, Cached: f.Cached,
		Degraded: f.Degraded, Source: f.Source, Items: items,
	})
}

// handleNotFound answers an unknown route with the service's BAD_REQUEST
// contract rather than the mux's plain-text 404.
func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	s.writeError(w, apperr.New(apperr.CodeBadRequest, 0,
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

// douyinItemResponse is one video in the JSON view.
type douyinItemResponse struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	URL       string    `json:"url"`
	HTML      string    `json:"html"`
	Images    []string  `json:"images"`
	CreatedAt time.Time `json:"created_at"`
}

type douyinResponse struct {
	User        string               `json:"user"`
	Title       string               `json:"title"`
	Description string               `json:"description"`
	FetchedAt   time.Time            `json:"fetched_at"`
	Cached      bool                 `json:"cached"`
	Degraded    bool                 `json:"degraded"`
	Items       []douyinItemResponse `json:"items"`
}

// handleDouyin renders one creator's timeline as RSS.
func (s *Server) handleDouyin(w http.ResponseWriter, r *http.Request) {
	sec := strings.TrimSuffix(r.PathValue("sec"), ".xml")
	limit := limitParam(r, feed.DefaultDouyinLimit, feed.MaxDouyinLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetDouyin(r.Context(), sec, limit, refresh)
	s.serveFeed(w, r, f, err, int(feed.DouyinTTL().Seconds()))
}

// handleAPIDouyin returns the same timeline as JSON.
func (s *Server) handleAPIDouyin(w http.ResponseWriter, r *http.Request) {
	sec := strings.TrimSpace(r.URL.Query().Get("user"))
	limit := limitParam(r, feed.DefaultDouyinLimit, feed.MaxDouyinLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetDouyin(r.Context(), sec, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	items := make([]douyinItemResponse, 0, len(f.Items))
	for _, it := range f.Items {
		items = append(items, douyinItemResponse{
			ID: it.ID, Title: it.Title, Text: it.Text, URL: it.URL,
			HTML: it.HTML, Images: it.Images, CreatedAt: it.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, douyinResponse{
		User: f.UID, Title: f.Title(), Description: f.Description,
		FetchedAt: f.FetchedAt, Cached: f.Cached, Degraded: f.Degraded, Items: items,
	})
}

// douyinTimelineItem is the downloader-facing shape: it keeps the identifiers
// and media addresses that the RSS view does not need.
type douyinTimelineItem struct {
	AwemeID    string `json:"aweme_id"`
	Desc       string `json:"desc"`
	Author     string `json:"author"`
	SecUID     string `json:"sec_uid"`
	CreateTime int64  `json:"create_time"`
	DurationMS int    `json:"duration_ms"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	IsTop      int    `json:"is_top"`
	PlayURL    string `json:"play_url"`
	CoverURL   string `json:"cover_url"`
}

type douyinTimelineResponse struct {
	Author    string               `json:"author"`
	SecUID    string               `json:"sec_uid"`
	FetchedAt time.Time            `json:"fetched_at"`
	Source    string               `json:"source"`
	Items     []douyinTimelineItem `json:"items"`
}

// handleAPIDouyinTimeline serves the timeline in the downloader's schema so a
// separate download tool can consume it without touching a browser.
func (s *Server) handleAPIDouyinTimeline(w http.ResponseWriter, r *http.Request) {
	sec := strings.TrimSpace(r.URL.Query().Get("user"))
	limit := limitParam(r, feed.DefaultDouyinLimit, feed.MaxDouyinLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetDouyin(r.Context(), sec, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	items := make([]douyinTimelineItem, 0, len(f.Items))
	for _, it := range f.Items {
		cover := ""
		if len(it.Images) > 0 {
			cover = it.Images[0]
		}
		top := 0
		if it.IsTop {
			top = 1
		}
		items = append(items, douyinTimelineItem{
			AwemeID: it.ID, Desc: it.Text, Author: it.Author, SecUID: f.UID,
			CreateTime: it.CreatedAt.Unix(), DurationMS: it.DurationMS,
			Width: it.Width, Height: it.Height, IsTop: top,
			PlayURL: it.PlayURL, CoverURL: cover,
		})
	}
	writeJSON(w, http.StatusOK, douyinTimelineResponse{
		Author: f.ScreenName, SecUID: f.UID, FetchedAt: f.FetchedAt, Source: "douyin", Items: items,
	})
}

// tiktokItemResponse is one video in the JSON view.
type tiktokItemResponse struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	URL       string    `json:"url"`
	HTML      string    `json:"html"`
	Images    []string  `json:"images"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

type tiktokResponse struct {
	User        string               `json:"user"`
	Title       string               `json:"title"`
	Description string               `json:"description"`
	FetchedAt   time.Time            `json:"fetched_at"`
	Cached      bool                 `json:"cached"`
	Degraded    bool                 `json:"degraded"`
	Items       []tiktokItemResponse `json:"items"`
}

// handleTikTok renders one creator's timeline as RSS.
func (s *Server) handleTikTok(w http.ResponseWriter, r *http.Request) {
	user := strings.TrimSuffix(r.PathValue("user"), ".xml")
	limit := limitParam(r, feed.DefaultTikTokLimit, feed.MaxTikTokLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetTikTok(r.Context(), user, limit, refresh)
	s.serveFeed(w, r, f, err, int(feed.TikTokTTL().Seconds()))
}

// handleAPITikTok returns the same timeline as JSON.
func (s *Server) handleAPITikTok(w http.ResponseWriter, r *http.Request) {
	user := strings.TrimSpace(r.URL.Query().Get("user"))
	limit := limitParam(r, feed.DefaultTikTokLimit, feed.MaxTikTokLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetTikTok(r.Context(), user, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	items := make([]tiktokItemResponse, 0, len(f.Items))
	for _, it := range f.Items {
		items = append(items, tiktokItemResponse{
			ID: it.ID, Title: it.Title, Text: it.Text, URL: it.URL,
			HTML: it.HTML, Images: it.Images, Author: it.Author, CreatedAt: it.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, tiktokResponse{
		User: f.UID, Title: f.Title(), Description: f.Description,
		FetchedAt: f.FetchedAt, Cached: f.Cached, Degraded: f.Degraded, Items: items,
	})
}

// tiktokTimelineItem is the downloader-facing shape: it keeps the identifiers
// and media addresses that the RSS view does not need. Play addresses are
// signed and expire within hours, so a consumer must re-read the endpoint.
type tiktokTimelineItem struct {
	AwemeID    string `json:"aweme_id"`
	Desc       string `json:"desc"`
	Author     string `json:"author"`
	CreateTime int64  `json:"create_time"`
	DurationMS int    `json:"duration_ms"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	IsTop      int    `json:"is_top"`
	PlayURL    string `json:"play_url"`
	CoverURL   string `json:"cover_url"`
}

type tiktokTimelineResponse struct {
	Author    string               `json:"author"`
	User      string               `json:"user"`
	FetchedAt time.Time            `json:"fetched_at"`
	Source    string               `json:"source"`
	Items     []tiktokTimelineItem `json:"items"`
}

// handleAPITikTokTimeline serves the timeline in the downloader's schema so a
// separate download tool can consume it without solving the WAF itself.
func (s *Server) handleAPITikTokTimeline(w http.ResponseWriter, r *http.Request) {
	user := strings.TrimSpace(r.URL.Query().Get("user"))
	limit := limitParam(r, feed.DefaultTikTokLimit, feed.MaxTikTokLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetTikTok(r.Context(), user, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	items := make([]tiktokTimelineItem, 0, len(f.Items))
	for _, it := range f.Items {
		cover := ""
		if len(it.Images) > 0 {
			cover = it.Images[0]
		}
		top := 0
		if it.IsTop {
			top = 1
		}
		items = append(items, tiktokTimelineItem{
			AwemeID: it.ID, Desc: it.Text, Author: it.Author,
			CreateTime: it.CreatedAt.Unix(), DurationMS: it.DurationMS,
			Width: it.Width, Height: it.Height, IsTop: top,
			PlayURL: it.PlayURL, CoverURL: cover,
		})
	}
	writeJSON(w, http.StatusOK, tiktokTimelineResponse{
		Author: f.ScreenName, User: f.UID, FetchedAt: f.FetchedAt, Source: "tiktok", Items: items,
	})
}

// bilibiliItemResponse is one upload in the JSON view.
type bilibiliItemResponse struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	URL       string    `json:"url"`
	HTML      string    `json:"html"`
	Images    []string  `json:"images"`
	Author    string    `json:"author"`
	Views     int64     `json:"views"`
	CreatedAt time.Time `json:"created_at"`
}

type bilibiliResponse struct {
	User        string                 `json:"user"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	FetchedAt   time.Time              `json:"fetched_at"`
	Cached      bool                   `json:"cached"`
	Degraded    bool                   `json:"degraded"`
	Items       []bilibiliItemResponse `json:"items"`
}

// handleBilibili renders one creator's video timeline as RSS.
func (s *Server) handleBilibili(w http.ResponseWriter, r *http.Request) {
	mid := strings.TrimSuffix(r.PathValue("mid"), ".xml")
	limit := limitParam(r, feed.DefaultBilibiliLimit, feed.MaxBilibiliLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetBilibili(r.Context(), mid, limit, refresh)
	s.serveFeed(w, r, f, err, int(feed.BilibiliTTL().Seconds()))
}

// handleAPIBilibili returns the same timeline as JSON.
func (s *Server) handleAPIBilibili(w http.ResponseWriter, r *http.Request) {
	mid := strings.TrimSpace(r.URL.Query().Get("user"))
	limit := limitParam(r, feed.DefaultBilibiliLimit, feed.MaxBilibiliLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetBilibili(r.Context(), mid, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	items := make([]bilibiliItemResponse, 0, len(f.Items))
	for _, it := range f.Items {
		items = append(items, bilibiliItemResponse{
			ID: it.ID, Title: it.Title, Text: it.Text, URL: it.URL,
			HTML: it.HTML, Images: it.Images, Author: it.Author,
			Views: it.Views, CreatedAt: it.CreatedAt,
		})
	}
	if f.Degraded {
		w.Header().Set("X-Feedmesh-Degraded", "1")
	}
	writeJSON(w, http.StatusOK, bilibiliResponse{
		User: f.UID, Title: f.Title(), Description: f.Description,
		FetchedAt: f.FetchedAt, Cached: f.Cached, Degraded: f.Degraded, Items: items,
	})
}

// telegramItemResponse is one post in the JSON view.
type telegramItemResponse struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	URL       string    `json:"url"`
	HTML      string    `json:"html"`
	Images    []string  `json:"images"`
	Author    string    `json:"author"`
	Views     int64     `json:"views"`
	CreatedAt time.Time `json:"created_at"`
}

type telegramResponse struct {
	Channel     string                 `json:"channel"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	FetchedAt   time.Time              `json:"fetched_at"`
	Cached      bool                   `json:"cached"`
	Degraded    bool                   `json:"degraded"`
	Items       []telegramItemResponse `json:"items"`
}

// handleTelegram renders one public channel preview as RSS.
func (s *Server) handleTelegram(w http.ResponseWriter, r *http.Request) {
	channel := strings.TrimSuffix(r.PathValue("channel"), ".xml")
	limit := limitParam(r, feed.DefaultTelegramLimit, feed.MaxTelegramLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetTelegram(r.Context(), channel, limit, refresh)
	s.serveFeed(w, r, f, err, int(feed.TelegramTTL().Seconds()))
}

// handleAPITelegram returns the same channel preview as JSON.
func (s *Server) handleAPITelegram(w http.ResponseWriter, r *http.Request) {
	channel := strings.TrimSpace(r.URL.Query().Get("channel"))
	limit := limitParam(r, feed.DefaultTelegramLimit, feed.MaxTelegramLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetTelegram(r.Context(), channel, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	items := make([]telegramItemResponse, 0, len(f.Items))
	for _, it := range f.Items {
		images := it.Images
		if images == nil {
			images = []string{}
		}
		items = append(items, telegramItemResponse{
			ID: it.ID, Title: it.Title, Text: it.Text, URL: it.URL,
			HTML: it.HTML, Images: images, Author: it.Author,
			Views: it.Views, CreatedAt: it.CreatedAt,
		})
	}
	if f.Degraded {
		w.Header().Set("X-Feedmesh-Degraded", "1")
	}
	writeJSON(w, http.StatusOK, telegramResponse{
		Channel: f.UID, Title: f.Title(), Description: f.Description,
		FetchedAt: f.FetchedAt, Cached: f.Cached, Degraded: f.Degraded, Items: items,
	})
}

// pinterestItemResponse is one pin in the JSON view.
type pinterestItemResponse struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	URL       string    `json:"url"`
	HTML      string    `json:"html"`
	Images    []string  `json:"images"`
	CreatedAt time.Time `json:"created_at"`
}

type pinterestResponse struct {
	User        string                  `json:"user"`
	Board       string                  `json:"board,omitempty"`
	Title       string                  `json:"title"`
	Description string                  `json:"description"`
	FetchedAt   time.Time               `json:"fetched_at"`
	Cached      bool                    `json:"cached"`
	Degraded    bool                    `json:"degraded"`
	Items       []pinterestItemResponse `json:"items"`
}

func pinterestItems(items []feed.Item) []pinterestItemResponse {
	out := make([]pinterestItemResponse, 0, len(items))
	for _, it := range items {
		images := it.Images
		if images == nil {
			images = []string{}
		}
		out = append(out, pinterestItemResponse{
			ID: it.ID, Title: it.Title, Text: it.Text, URL: it.URL,
			HTML: it.HTML, Images: images, CreatedAt: it.CreatedAt,
		})
	}
	return out
}

// handlePinterest serves a profile feed or, with a second path segment, one
// board feed.
func (s *Server) handlePinterest(w http.ResponseWriter, r *http.Request) {
	user := strings.TrimSuffix(r.PathValue("user"), ".xml")
	board := strings.TrimSuffix(r.PathValue("board"), ".xml")
	limit := limitParam(r, feed.DefaultPinterestLimit, feed.MaxPinterestLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetPinterest(r.Context(), user, board, limit, refresh)
	s.serveFeed(w, r, f, err, int(feed.PinterestTTL().Seconds()))
}

// handleAPIPinterest returns the same profile or board feed as JSON.
func (s *Server) handleAPIPinterest(w http.ResponseWriter, r *http.Request) {
	user := strings.TrimSpace(r.URL.Query().Get("user"))
	board := strings.TrimSpace(r.URL.Query().Get("board"))
	limit := limitParam(r, feed.DefaultPinterestLimit, feed.MaxPinterestLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetPinterest(r.Context(), user, board, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	if f.Degraded {
		w.Header().Set("X-Feedmesh-Degraded", "1")
	}
	respUser, respBoard := f.UID, ""
	if i := strings.Index(f.UID, "/"); i >= 0 {
		respUser, respBoard = f.UID[:i], f.UID[i+1:]
	}
	writeJSON(w, http.StatusOK, pinterestResponse{
		User: respUser, Board: respBoard, Title: f.Title(), Description: f.Description,
		FetchedAt: f.FetchedAt, Cached: f.Cached, Degraded: f.Degraded,
		Items: pinterestItems(f.Items),
	})
}

// polymarketItemResponse is one event, market or wallet action in the JSON view.
type polymarketItemResponse struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	URL       string    `json:"url"`
	HTML      string    `json:"html"`
	Images    []string  `json:"images"`
	CreatedAt time.Time `json:"created_at"`
	Volume24H float64   `json:"volume_24h,omitempty"`
	Liquidity float64   `json:"liquidity,omitempty"`
	EndsAt    time.Time `json:"ends_at,omitzero"`
}

type polymarketResponse struct {
	Feed        string                   `json:"feed"`
	Tag         string                   `json:"tag,omitempty"`
	Slug        string                   `json:"slug,omitempty"`
	Address     string                   `json:"address,omitempty"`
	Title       string                   `json:"title"`
	Description string                   `json:"description"`
	FetchedAt   time.Time                `json:"fetched_at"`
	Cached      bool                     `json:"cached"`
	Degraded    bool                     `json:"degraded"`
	Items       []polymarketItemResponse `json:"items"`
}

func polymarketItems(items []feed.Item) []polymarketItemResponse {
	out := make([]polymarketItemResponse, 0, len(items))
	for _, it := range items {
		images := it.Images
		if images == nil {
			images = []string{}
		}
		out = append(out, polymarketItemResponse{
			ID: it.ID, Title: it.Title, Text: it.Text, URL: it.URL,
			HTML: it.HTML, Images: images, CreatedAt: it.CreatedAt,
			Volume24H: it.Volume24H, Liquidity: it.Liquidity, EndsAt: it.EndsAt,
		})
	}
	return out
}

func (s *Server) servePolymarketRSS(w http.ResponseWriter, r *http.Request, f *feed.Feed, kind string) {
	s.serveFeed(w, r, f, nil, int(feed.PolymarketTTL(kind).Seconds()))
}

// handlePolymarketNew serves the newest markets.
func (s *Server) handlePolymarketNew(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimSpace(r.URL.Query().Get("tag"))
	limit := limitParam(r, feed.DefaultPolymarketLimit, feed.MaxPolymarketLimit)
	refresh := refreshParam(r)
	f, err := s.cfg.Service.GetPolymarketEvents(r.Context(), "createdAt", tag, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.servePolymarketRSS(w, r, f, "events")
}

// handlePolymarketHot serves events ranked by 24-hour volume.
func (s *Server) handlePolymarketHot(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimSpace(r.URL.Query().Get("tag"))
	limit := limitParam(r, feed.DefaultPolymarketLimit, feed.MaxPolymarketLimit)
	refresh := refreshParam(r)
	f, err := s.cfg.Service.GetPolymarketEvents(r.Context(), "volume24hr", tag, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.servePolymarketRSS(w, r, f, "events")
}

// handlePolymarketEvent serves one event's markets.
func (s *Server) handlePolymarketEvent(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSuffix(r.PathValue("slug"), ".xml")
	limit := limitParam(r, feed.DefaultPolymarketLimit, feed.MaxPolymarketLimit)
	refresh := refreshParam(r)
	f, err := s.cfg.Service.GetPolymarketEvent(r.Context(), slug, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.servePolymarketRSS(w, r, f, "event")
}

// handlePolymarketUser serves one wallet's activity.
func (s *Server) handlePolymarketUser(w http.ResponseWriter, r *http.Request) {
	address := strings.TrimSuffix(r.PathValue("address"), ".xml")
	limit := limitParam(r, feed.DefaultPolymarketActivityLimit, feed.MaxPolymarketActivityLimit)
	refresh := refreshParam(r)
	f, err := s.cfg.Service.GetPolymarketActivity(r.Context(), address, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.servePolymarketRSS(w, r, f, "activity")
}

// handleAPIPolymarket returns any Polymarket feed as JSON.
func (s *Server) handleAPIPolymarket(w http.ResponseWriter, r *http.Request) {
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("feed")))
	if kind == "" {
		kind = "new"
	}
	tag := strings.TrimSpace(r.URL.Query().Get("tag"))
	slug := strings.TrimSpace(r.URL.Query().Get("slug"))
	address := strings.TrimSpace(r.URL.Query().Get("address"))
	refresh := refreshParam(r)

	var (
		f     *feed.Feed
		limit int
		err   error
	)
	switch kind {
	case "new", "hot":
		order := "createdAt"
		if kind == "hot" {
			order = "volume24hr"
		}
		limit = limitParam(r, feed.DefaultPolymarketLimit, feed.MaxPolymarketLimit)
		f, err = s.cfg.Service.GetPolymarketEvents(r.Context(), order, tag, limit, refresh)
	case "event":
		limit = limitParam(r, feed.DefaultPolymarketLimit, feed.MaxPolymarketLimit)
		f, err = s.cfg.Service.GetPolymarketEvent(r.Context(), slug, limit, refresh)
	case "user", "activity":
		kind = "user"
		limit = limitParam(r, feed.DefaultPolymarketActivityLimit, feed.MaxPolymarketActivityLimit)
		f, err = s.cfg.Service.GetPolymarketActivity(r.Context(), address, limit, refresh)
	default:
		s.writeErrorStatus(w, http.StatusBadRequest,
			apperr.New(apperr.CodeBadRequest, 0, "unknown polymarket feed %q", kind))
		return
	}
	if err != nil {
		s.writeError(w, err)
		return
	}
	if f.Degraded {
		w.Header().Set("X-Feedmesh-Degraded", "1")
	}
	writeJSON(w, http.StatusOK, polymarketResponse{
		Feed: kind, Tag: tag, Slug: slug, Address: address,
		Title: f.Title(), Description: f.Description,
		FetchedAt: f.FetchedAt, Cached: f.Cached, Degraded: f.Degraded,
		Items: polymarketItems(f.Items),
	})
}

// linuxdoItemResponse is one forum topic in the JSON view.
type linuxdoItemResponse struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	URL       string    `json:"url"`
	HTML      string    `json:"html"`
	Images    []string  `json:"images"`
	Author    string    `json:"author"`
	Category  string    `json:"category"`
	Pinned    bool      `json:"pinned"`
	Closed    bool      `json:"closed"`
	Archived  bool      `json:"archived"`
	CreatedAt time.Time `json:"created_at"`
}

type linuxdoResponse struct {
	Feed        string                `json:"feed"`
	Title       string                `json:"title"`
	Description string                `json:"description"`
	FetchedAt   time.Time             `json:"fetched_at"`
	Cached      bool                  `json:"cached"`
	Degraded    bool                  `json:"degraded"`
	Items       []linuxdoItemResponse `json:"items"`
}

// serveLinuxDoRSS renders one forum feed as RSS.
func (s *Server) serveLinuxDoRSS(w http.ResponseWriter, r *http.Request, ref linuxdo.Ref) {
	limit := limitParam(r, feed.DefaultLinuxDoLimit, feed.MaxLinuxDoLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetLinuxDo(r.Context(), ref, limit, refresh)
	s.serveFeed(w, r, f, err, int(feed.LinuxDoTTL().Seconds()))
}

// handleLinuxDoLatest serves the latest-topics feed.
func (s *Server) handleLinuxDoLatest(w http.ResponseWriter, r *http.Request) {
	s.serveLinuxDoRSS(w, r, linuxdo.Ref{Kind: linuxdo.KindLatest, Order: r.URL.Query().Get("order")})
}

// handleLinuxDoTop serves the ranked top feed for one period.
func (s *Server) handleLinuxDoTop(w http.ResponseWriter, r *http.Request) {
	s.serveLinuxDoRSS(w, r, linuxdo.Ref{Kind: linuxdo.KindTop, Period: r.URL.Query().Get("period")})
}

// handleLinuxDoHot serves the ranked hot feed.
func (s *Server) handleLinuxDoHot(w http.ResponseWriter, r *http.Request) {
	s.serveLinuxDoRSS(w, r, linuxdo.Ref{Kind: linuxdo.KindHot})
}

// handleLinuxDoCategory serves /c/<slug>/<id> and the id-only redirect form.
func (s *Server) handleLinuxDoCategory(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	id := strings.TrimSuffix(r.PathValue("id"), ".xml")
	if slug == "-" {
		slug = ""
	}
	s.serveLinuxDoRSS(w, r, linuxdo.Ref{Kind: linuxdo.KindCategory, Slug: slug, ID: id})
}

// handleLinuxDoTag serves one forum tag.
func (s *Server) handleLinuxDoTag(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimSuffix(r.PathValue("tag"), ".xml")
	s.serveLinuxDoRSS(w, r, linuxdo.Ref{Kind: linuxdo.KindTag, Tag: tag})
}

// handleAPILinuxDo returns the same feed as JSON.
func (s *Server) handleAPILinuxDo(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind := strings.ToLower(strings.TrimSpace(q.Get("feed")))
	if kind == "" {
		kind = string(linuxdo.KindLatest)
	}
	ref := linuxdo.Ref{Kind: linuxdo.Kind(kind)}
	switch ref.Kind {
	case linuxdo.KindLatest:
		ref.Order = q.Get("order")
	case linuxdo.KindTop:
		ref.Period = q.Get("period")
	case linuxdo.KindHot:
	case linuxdo.KindCategory:
		raw := strings.TrimSpace(q.Get("category"))
		if raw == "" {
			s.writeError(w, apperr.New(apperr.CodeBadRequest, 0, "missing category"))
			return
		}
		if i := strings.LastIndex(raw, "/"); i >= 0 {
			ref.Slug, ref.ID = raw[:i], raw[i+1:]
		} else {
			ref.ID = raw
		}
	case linuxdo.KindTag:
		ref.Tag = q.Get("tag")
	default:
		s.writeError(w, apperr.New(apperr.CodeBadRequest, 0, "unknown linux.do feed %q", kind))
		return
	}

	limit := atoiClamp(q.Get("limit"), feed.DefaultLinuxDoLimit, feed.MaxLinuxDoLimit)
	refresh := q.Get("refresh") == "1"
	f, err := s.cfg.Service.GetLinuxDo(r.Context(), ref, limit, refresh)
	if err != nil {
		s.writeError(w, err)
		return
	}
	items := make([]linuxdoItemResponse, 0, len(f.Items))
	for _, it := range f.Items {
		images := it.Images
		if images == nil {
			images = []string{}
		}
		items = append(items, linuxdoItemResponse{
			ID: it.ID, Title: it.Title, Text: it.Text, URL: it.URL, HTML: it.HTML,
			Images: images, Author: it.Author, Category: it.Category,
			Pinned: it.Pinned, Closed: it.Closed, Archived: it.Archived,
			CreatedAt: it.CreatedAt,
		})
	}
	if f.Degraded {
		w.Header().Set("X-Feedmesh-Degraded", "1")
	}
	writeJSON(w, http.StatusOK, linuxdoResponse{
		Feed: f.UID, Title: f.Title(), Description: f.Description,
		FetchedAt: f.FetchedAt, Cached: f.Cached, Degraded: f.Degraded, Items: items,
	})
}
