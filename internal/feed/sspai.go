package feed

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yilong-musk/feedmesh/internal/sspai"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

// sspai feed bounds and cache lifetimes.
const (
	DefaultSspaiLimit = 10
	MaxSspaiLimit     = sspai.MaxLimit
	sspaiListTTL      = 10 * time.Minute
	sspaiDetailTTL    = 24 * time.Hour
)

var (
	sspaiScript = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	sspaiStyle  = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
)

// GetSspai builds the sspai feed. full controls whether article bodies are
// fetched (cached 24h per article); without it items carry the summary only.
func (s *Service) GetSspai(ctx context.Context, limit int, full, refresh bool) (*Feed, error) {
	if s.clients.Sspai == nil {
		return nil, &weibo.Error{Code: weibo.CodeInternal, Message: "sspai provider is not configured"}
	}
	if limit <= 0 {
		limit = DefaultSspaiLimit
	}
	if limit > MaxSspaiLimit {
		limit = MaxSspaiLimit
	}

	articles, hit, err := fetchCached(ctx, s, "sspai:list", sspaiListTTL, refresh,
		func() ([]sspai.Article, error) { return s.clients.Sspai.List(ctx, limit) })
	if err != nil {
		return nil, sspaiError(err)
	}
	if len(articles) > limit {
		articles = articles[:limit]
	}

	now := s.now().UTC()
	f := &Feed{
		TitleText:   "少数派",
		LinkText:    "https://sspai.com",
		Description: "少数派最新文章",
		FetchedAt:   now,
		Cached:      hit,
		TTL:         15,
		Items:       make([]Item, 0, len(articles)),
	}
	if full {
		// Detail calls dominate latency (~2s each upstream), so fetch a few
		// in parallel. Three workers keep a cold 10-item feed around 15-25s while
		// staying polite to sspai (the detail API is slow: 3-20s per call).
		detailed := make([]sspai.Article, len(articles))
		sem := make(chan struct{}, 3)
		var wg sync.WaitGroup
		for i := range articles {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				a := articles[i]
				d, _, derr := fetchCached(ctx, s, "sspai:detail:"+strconv.Itoa(a.ID), sspaiDetailTTL, refresh,
					func() (sspai.Article, error) { return s.clients.Sspai.Detail(ctx, a.ID) })
				if derr != nil {
					s.log.Warn("sspai detail unavailable, keeping summary",
						"id", a.ID, "error", derr.Error())
					detailed[i] = a
					return
				}
				a.Body = d.Body
				a.Keywords = d.Keywords
				a.IsPay = d.IsPay
				detailed[i] = a
			}(i)
		}
		wg.Wait()
		articles = detailed
	}

	for _, a := range articles {

		created := now
		if a.ReleasedTime > 0 {
			created = time.Unix(a.ReleasedTime, 0).UTC()
		}
		images := []string{}
		if cover := a.CoverURL(); cover != "" {
			images = append(images, cover)
		}
		f.Items = append(f.Items, Item{
			ID:        strconv.Itoa(a.ID),
			Title:     a.Title,
			Text:      a.Summary,
			HTML:      sspaiHTML(a, full),
			URL:       a.URL(),
			CreatedAt: created,
			Images:    images,
		})
	}
	return f, nil
}

// sspaiHTML renders the item body: cover, author/keywords, then the full body
// (or the summary plus a read-more link).
func sspaiHTML(a sspai.Article, full bool) string {
	var b strings.Builder
	if cover := a.CoverURL(); cover != "" {
		writeImage(&b, cover, "", "")
	}

	meta := []string{}
	if who := strings.TrimSpace(a.Author.Nickname); who != "" {
		meta = append(meta, who)
	}
	if kw := strings.TrimSpace(a.Keywords); kw != "" {
		meta = append(meta, kw)
	}
	if len(meta) > 0 {
		b.WriteString("<p>")
		b.WriteString(EscapeXML(strings.Join(meta, " · ")))
		b.WriteString("</p>")
	}

	if a.IsPay {
		b.WriteString("<p><em>会员文章，仅显示摘要。</em></p>")
	}
	if full && strings.TrimSpace(a.Body) != "" {
		body := sspaiStyle.ReplaceAllString(a.Body, "")
		body = sspaiScript.ReplaceAllString(body, "")
		b.WriteString(body)
		return b.String()
	}
	if sum := strings.TrimSpace(a.Summary); sum != "" {
		b.WriteString("<p>")
		b.WriteString(EscapeXML(sum))
		b.WriteString("</p>")
	}
	b.WriteString(`<p><a href="`)
	b.WriteString(EscapeXML(a.URL()))
	b.WriteString(`">阅读全文</a></p>`)
	return b.String()
}

// sspaiError keeps weibo-shaped codes intact and wraps the rest as upstream
// failures so the HTTP layer answers 502 instead of 500.
func sspaiError(err error) error {
	if err == nil {
		return nil
	}
	var we *weibo.Error
	if errors.As(err, &we) {
		return err
	}
	return &weibo.Error{Code: weibo.CodeUpstreamUnavailable, Message: fmt.Sprintf("sspai: %v", err), Err: err}
}
