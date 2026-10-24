package feed

import (
	"context"
	"errors"
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

	// Cache the list, not the caller's slice: fetchCachedList widens the entry
	// when a later request asks for more, so ?limit=3 can never truncate the
	// rest of the TTL. The loader pages upstream only as wide as needed.
	articles, hit, err := fetchCachedList(ctx, s, "sspai:list", sspaiListTTL, refresh, limit,
		func(n int) ([]sspai.Article, error) { return s.clients.Sspai.List(ctx, n) })
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
		// Detail calls are network-bound; a cold 30-item feed measured around
		// 4.5s end to end with three workers. Keep the parallelism modest so a
		// burst of feed readers stays polite to sspai.
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
		if cover := safeURLString(a.CoverURL()); cover != "" {
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
		// Never republish a paywalled body, even if upstream accidentally
		// returned one.
		b.WriteString("<p><em>会员文章，仅显示摘要。</em></p>")
	} else if full && strings.TrimSpace(a.Body) != "" {
		b.WriteString(NormalizeHTMLBase(a.Body, "https://sspai.com"))
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
// failures (HTTP 504). The message is provider-neutral because X and V2EX
// share it.
func sspaiError(err error) error {
	if err == nil {
		return nil
	}
	var we *weibo.Error
	if errors.As(err, &we) {
		return err
	}
	return &weibo.Error{Code: weibo.CodeUpstreamUnavailable, Message: err.Error(), Err: err}
}
