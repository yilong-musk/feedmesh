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

// degradedRSSMaxAge is the reader-facing lifetime of any document served from
// its last good cache entry: short enough that readers pick the real feed up
// soon after the upstream recovers, even though the content may be hours old.
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
	mux.HandleFunc("/rss/apple/top", s.getOnly(s.handleAppleChart))
	mux.HandleFunc("/rss/apple/top.xml", s.getOnly(s.handleAppleChart))
	mux.HandleFunc("/rss/apple/{id}", s.getOnly(s.handleApple))
	mux.HandleFunc("/rss/xiaoyuzhou/{id}", s.getOnly(s.handleXiaoyuzhou))
	mux.HandleFunc("/rss/trumpstruth", s.getOnly(s.handleTrumpTruth))
	mux.HandleFunc("/rss/trumpstruth.xml", s.getOnly(s.handleTrumpTruth))
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
	mux.HandleFunc("/api/v1/apple", s.getOnly(s.handleAPIApple))
	mux.HandleFunc("/api/v1/xiaoyuzhou", s.getOnly(s.handleAPIXiaoyuzhou))
	mux.HandleFunc("/api/v1/trumpstruth", s.getOnly(s.handleAPITrumpTruth))
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

// setSource publishes where a list came from: atom (the official document) or
// stale (the last good cached list). It is the only signal an operator gets
// that an outage window or a rate-limit window is in progress.
func setSource(w http.ResponseWriter, source string) {
	if source != "" {
		w.Header().Set("X-Feedmesh-Source", source)
	}
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

// ----------------------------------------------------------------------- Weibo

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

func (s *Server) handleAPIFeed(w http.ResponseWriter, r *http.Request) {
	uid := strings.TrimSpace(r.URL.Query().Get("uid"))
	if uid == "" {
		s.writeError(w, apperr.New(apperr.CodeBadRequest, 0, "query parameter uid is required"))
		return
	}
	limit := atoiDefault(r.URL.Query().Get("limit"), feed.DefaultLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetFeed(r.Context(), uid, limit, refresh)
	var meta map[string]any
	if f != nil {
		meta = map[string]any{"uid": f.UID, "screen_name": f.ScreenName}
	}
	s.serveJSON(w, "weibo", f, err, meta)
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
	s.serveJSON(w, "hot", f, err, nil)
}

// ----------------------------------------------------------------------- sspai

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
	s.serveJSON(w, "sspai", f, err, nil)
}

// ------------------------------------------------------------------------ V2EX

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
	s.serveJSON(w, "v2ex", f, err, map[string]any{"section": section})
}

// ----------------------------------------------------------------------- SMZDM

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
		s.serveJSON(w, "smzdm", f, err, map[string]any{"channel": string(ch), "slug": strings.ToLower(slug)})
		return
	}

	f, err := s.cfg.Service.GetSmzdm(r.Context(), ch, limit, refresh)
	s.serveJSON(w, "smzdm", f, err, map[string]any{"channel": string(ch)})
}

// --------------------------------------------------------------------------- X

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
	var meta map[string]any
	if f != nil {
		meta = map[string]any{"handle": f.UID}
	}
	s.serveJSON(w, "x", f, err, meta)
}

// --------------------------------------------------------------------- YouTube

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
	var meta map[string]any
	if f != nil {
		meta = map[string]any{"ref": ref, "channel_id": f.UID}
	}
	s.serveJSON(w, "youtube", f, err, meta)
}

// ---------------------------------------------------------------------- Reddit

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
	var meta map[string]any
	if f != nil {
		meta = map[string]any{"sub": f.UID, "sort": string(sort), "window": string(window)}
	}
	s.serveJSON(w, "reddit", f, err, meta)
}

// ---------------------------------------------------------------------- Douyin

// handleDouyin renders one creator's timeline as RSS.
func (s *Server) handleDouyin(w http.ResponseWriter, r *http.Request) {
	sec := strings.TrimSuffix(r.PathValue("sec"), ".xml")
	limit := limitParam(r, feed.DefaultDouyinLimit, feed.MaxDouyinLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetDouyin(r.Context(), sec, limit, refresh)
	s.serveFeed(w, r, f, err, int(feed.DouyinTTL().Seconds()))
}

// handleAPIDouyin returns the same timeline in the unified JSON envelope.
func (s *Server) handleAPIDouyin(w http.ResponseWriter, r *http.Request) {
	sec := strings.TrimSpace(r.URL.Query().Get("user"))
	limit := limitParam(r, feed.DefaultDouyinLimit, feed.MaxDouyinLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetDouyin(r.Context(), sec, limit, refresh)
	var meta map[string]any
	if f != nil {
		meta = map[string]any{"user": f.UID}
	}
	s.serveJSON(w, "douyin", f, err, meta)
}

// douyinTimelineItem is the downloader-facing shape: it keeps the identifiers
// and media addresses that the feed views do not need.
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
		var dl feed.DownloadMedia
		if it.Download != nil {
			dl = *it.Download
		}
		top := 0
		if dl.IsTop {
			top = 1
		}
		items = append(items, douyinTimelineItem{
			AwemeID: it.ID, Desc: it.Text, Author: it.Author, SecUID: f.UID,
			CreateTime: it.CreatedAt.Unix(), DurationMS: dl.DurationMS,
			Width: dl.Width, Height: dl.Height, IsTop: top,
			PlayURL: dl.PlayURL, CoverURL: cover,
		})
	}
	writeJSON(w, http.StatusOK, douyinTimelineResponse{
		Author: f.ScreenName, SecUID: f.UID, FetchedAt: f.FetchedAt, Source: "douyin", Items: items,
	})
}

// ---------------------------------------------------------------------- TikTok

// handleTikTok renders one creator's timeline as RSS.
func (s *Server) handleTikTok(w http.ResponseWriter, r *http.Request) {
	user := strings.TrimSuffix(r.PathValue("user"), ".xml")
	limit := limitParam(r, feed.DefaultTikTokLimit, feed.MaxTikTokLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetTikTok(r.Context(), user, limit, refresh)
	s.serveFeed(w, r, f, err, int(feed.TikTokTTL().Seconds()))
}

// handleAPITikTok returns the same timeline in the unified JSON envelope.
func (s *Server) handleAPITikTok(w http.ResponseWriter, r *http.Request) {
	user := strings.TrimSpace(r.URL.Query().Get("user"))
	limit := limitParam(r, feed.DefaultTikTokLimit, feed.MaxTikTokLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetTikTok(r.Context(), user, limit, refresh)
	var meta map[string]any
	if f != nil {
		meta = map[string]any{"user": f.UID}
	}
	s.serveJSON(w, "tiktok", f, err, meta)
}

// tiktokTimelineItem is the downloader-facing shape: it keeps the identifiers
// and media addresses that the feed views do not need. Play addresses are
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
		var dl feed.DownloadMedia
		if it.Download != nil {
			dl = *it.Download
		}
		top := 0
		if dl.IsTop {
			top = 1
		}
		items = append(items, tiktokTimelineItem{
			AwemeID: it.ID, Desc: it.Text, Author: it.Author,
			CreateTime: it.CreatedAt.Unix(), DurationMS: dl.DurationMS,
			Width: dl.Width, Height: dl.Height, IsTop: top,
			PlayURL: dl.PlayURL, CoverURL: cover,
		})
	}
	writeJSON(w, http.StatusOK, tiktokTimelineResponse{
		Author: f.ScreenName, User: f.UID, FetchedAt: f.FetchedAt, Source: "tiktok", Items: items,
	})
}

// -------------------------------------------------------------------- Bilibili

// handleBilibili renders one creator's video timeline as RSS.
func (s *Server) handleBilibili(w http.ResponseWriter, r *http.Request) {
	mid := strings.TrimSuffix(r.PathValue("mid"), ".xml")
	limit := limitParam(r, feed.DefaultBilibiliLimit, feed.MaxBilibiliLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetBilibili(r.Context(), mid, limit, refresh)
	s.serveFeed(w, r, f, err, int(feed.BilibiliTTL().Seconds()))
}

// handleAPIBilibili returns the same timeline in the unified JSON envelope.
func (s *Server) handleAPIBilibili(w http.ResponseWriter, r *http.Request) {
	mid := strings.TrimSpace(r.URL.Query().Get("user"))
	limit := limitParam(r, feed.DefaultBilibiliLimit, feed.MaxBilibiliLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetBilibili(r.Context(), mid, limit, refresh)
	var meta map[string]any
	if f != nil {
		meta = map[string]any{"user": f.UID}
	}
	s.serveJSON(w, "bilibili", f, err, meta)
}

// -------------------------------------------------------------------- Telegram

// handleTelegram renders one public channel preview as RSS.
func (s *Server) handleTelegram(w http.ResponseWriter, r *http.Request) {
	channel := strings.TrimSuffix(r.PathValue("channel"), ".xml")
	limit := limitParam(r, feed.DefaultTelegramLimit, feed.MaxTelegramLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetTelegram(r.Context(), channel, limit, refresh)
	s.serveFeed(w, r, f, err, int(feed.TelegramTTL().Seconds()))
}

// handleAPITelegram returns the same channel preview in the unified JSON envelope.
func (s *Server) handleAPITelegram(w http.ResponseWriter, r *http.Request) {
	channel := strings.TrimSpace(r.URL.Query().Get("channel"))
	limit := limitParam(r, feed.DefaultTelegramLimit, feed.MaxTelegramLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetTelegram(r.Context(), channel, limit, refresh)
	var meta map[string]any
	if f != nil {
		meta = map[string]any{"channel": f.UID}
	}
	s.serveJSON(w, "telegram", f, err, meta)
}

// --------------------------------------------------------------------- Apple

// handleApple renders one Apple Podcasts show as RSS.
func (s *Server) handleApple(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(r.PathValue("id"), ".xml")
	limit := limitParam(r, feed.DefaultAppleLimit, feed.MaxAppleLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetApple(r.Context(), id, limit, refresh)
	s.serveFeed(w, r, f, err, int(feed.AppleTTL().Seconds()))
}

// handleAppleChart renders one storefront's top shows as RSS.
func (s *Server) handleAppleChart(w http.ResponseWriter, r *http.Request) {
	country := r.URL.Query().Get("country")
	limit := limitParam(r, feed.DefaultAppleLimit, feed.MaxAppleLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetAppleChart(r.Context(), country, limit, refresh)
	s.serveFeed(w, r, f, err, int(feed.AppleTTL().Seconds()))
}

// handleAPIApple returns a show or, with chart=top, a top-shows list.
func (s *Server) handleAPIApple(w http.ResponseWriter, r *http.Request) {
	limit := limitParam(r, feed.DefaultAppleLimit, feed.MaxAppleLimit)
	refresh := refreshParam(r)
	if strings.TrimSpace(r.URL.Query().Get("chart")) == "top" {
		country := r.URL.Query().Get("country")
		f, err := s.cfg.Service.GetAppleChart(r.Context(), country, limit, refresh)
		var meta map[string]any
		if f != nil {
			meta = map[string]any{"chart": "top", "country": f.UID}
		}
		s.serveJSON(w, "apple", f, err, meta)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	f, err := s.cfg.Service.GetApple(r.Context(), id, limit, refresh)
	var meta map[string]any
	if f != nil {
		meta = map[string]any{"id": f.UID}
	}
	s.serveJSON(w, "apple", f, err, meta)
}

// ----------------------------------------------------------------- Xiaoyuzhou

// handleXiaoyuzhou renders one Xiaoyuzhou show as RSS.
func (s *Server) handleXiaoyuzhou(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(r.PathValue("id"), ".xml")
	limit := limitParam(r, feed.DefaultXiaoyuzhouLimit, feed.MaxXiaoyuzhouLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetXiaoyuzhou(r.Context(), id, limit, refresh)
	s.serveFeed(w, r, f, err, int(feed.XiaoyuzhouTTL().Seconds()))
}

// handleAPIXiaoyuzhou returns the same show in the unified JSON envelope.
func (s *Server) handleAPIXiaoyuzhou(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	limit := limitParam(r, feed.DefaultXiaoyuzhouLimit, feed.MaxXiaoyuzhouLimit)
	refresh := refreshParam(r)

	f, err := s.cfg.Service.GetXiaoyuzhou(r.Context(), id, limit, refresh)
	var meta map[string]any
	if f != nil {
		meta = map[string]any{"id": f.UID}
	}
	s.serveJSON(w, "xiaoyuzhou", f, err, meta)
}

// ---------------------------------------------------------------- Trump's Truth

// handleTrumpTruth renders the archive of Donald Trump's Truth Social posts.
func (s *Server) handleTrumpTruth(w http.ResponseWriter, r *http.Request) {
	limit := limitParam(r, feed.DefaultTrumpTruthLimit, feed.MaxTrumpTruthLimit)
	refresh := refreshParam(r)
	f, err := s.cfg.Service.GetTrumpTruth(r.Context(), limit, refresh)
	s.serveFeed(w, r, f, err, int(feed.TrumpTruthTTL().Seconds()))
}

// handleAPITrumpTruth returns the same archive feed in the unified JSON envelope.
func (s *Server) handleAPITrumpTruth(w http.ResponseWriter, r *http.Request) {
	limit := limitParam(r, feed.DefaultTrumpTruthLimit, feed.MaxTrumpTruthLimit)
	refresh := refreshParam(r)
	f, err := s.cfg.Service.GetTrumpTruth(r.Context(), limit, refresh)
	s.serveJSON(w, "trumpstruth", f, err, nil)
}

// ------------------------------------------------------------------- Pinterest

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
	var meta map[string]any
	if f != nil {
		respUser, respBoard := f.UID, ""
		if i := strings.Index(f.UID, "/"); i >= 0 {
			respUser, respBoard = f.UID[:i], f.UID[i+1:]
		}
		meta = map[string]any{"user": respUser}
		if respBoard != "" {
			meta["board"] = respBoard
		}
	}
	s.serveJSON(w, "pinterest", f, err, meta)
}

// ------------------------------------------------------------------ Polymarket

func (s *Server) servePolymarketRSS(w http.ResponseWriter, r *http.Request, f *feed.Feed, err error, kind string) {
	s.serveFeed(w, r, f, err, int(feed.PolymarketTTL(kind).Seconds()))
}

// handlePolymarketNew serves the newest markets.
func (s *Server) handlePolymarketNew(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimSpace(r.URL.Query().Get("tag"))
	limit := limitParam(r, feed.DefaultPolymarketLimit, feed.MaxPolymarketLimit)
	refresh := refreshParam(r)
	f, err := s.cfg.Service.GetPolymarketEvents(r.Context(), "createdAt", tag, limit, refresh)
	s.servePolymarketRSS(w, r, f, err, "events")
}

// handlePolymarketHot serves events ranked by 24-hour volume.
func (s *Server) handlePolymarketHot(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimSpace(r.URL.Query().Get("tag"))
	limit := limitParam(r, feed.DefaultPolymarketLimit, feed.MaxPolymarketLimit)
	refresh := refreshParam(r)
	f, err := s.cfg.Service.GetPolymarketEvents(r.Context(), "volume24hr", tag, limit, refresh)
	s.servePolymarketRSS(w, r, f, err, "events")
}

// handlePolymarketEvent serves one event's markets.
func (s *Server) handlePolymarketEvent(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSuffix(r.PathValue("slug"), ".xml")
	limit := limitParam(r, feed.DefaultPolymarketLimit, feed.MaxPolymarketLimit)
	refresh := refreshParam(r)
	f, err := s.cfg.Service.GetPolymarketEvent(r.Context(), slug, limit, refresh)
	s.servePolymarketRSS(w, r, f, err, "event")
}

// handlePolymarketUser serves one wallet's activity.
func (s *Server) handlePolymarketUser(w http.ResponseWriter, r *http.Request) {
	address := strings.TrimSuffix(r.PathValue("address"), ".xml")
	limit := limitParam(r, feed.DefaultPolymarketActivityLimit, feed.MaxPolymarketActivityLimit)
	refresh := refreshParam(r)
	f, err := s.cfg.Service.GetPolymarketActivity(r.Context(), address, limit, refresh)
	s.servePolymarketRSS(w, r, f, err, "activity")
}

// handleAPIPolymarket returns any Polymarket feed in the unified JSON envelope.
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
	meta := map[string]any{"feed": kind}
	if tag != "" {
		meta["tag"] = tag
	}
	if slug != "" {
		meta["slug"] = slug
	}
	if address != "" {
		meta["address"] = address
	}
	s.serveJSON(w, "polymarket", f, err, meta)
}

// -------------------------------------------------------------------- LINUX DO

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

// handleAPILinuxDo returns the same feed in the unified JSON envelope.
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

	limit := limitParam(r, feed.DefaultLinuxDoLimit, feed.MaxLinuxDoLimit)
	refresh := refreshParam(r)
	f, err := s.cfg.Service.GetLinuxDo(r.Context(), ref, limit, refresh)
	var meta map[string]any
	if f != nil {
		meta = map[string]any{"feed": f.UID}
	}
	s.serveJSON(w, "linuxdo", f, err, meta)
}

// --------------------------------------------------------------------- helpers

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
