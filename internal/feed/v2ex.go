package feed

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/v2ex"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

// V2EX feed bounds and cache lifetime.
const (
	DefaultV2exLimit = 20
	MaxV2exLimit     = v2ex.MaxLimit
	v2exTTL          = 10 * time.Minute
)

var v2exImg = regexp.MustCompile(`(?i)<img[^>]+src="([^"]+)"`)

// GetV2ex builds a V2EX feed. section is "latest" (official Atom) or "hot"
// (the hot tab, sorted by replies).
func (s *Service) GetV2ex(ctx context.Context, section string, limit int, refresh bool) (*Feed, error) {
	if s.clients.V2ex == nil {
		return nil, &weibo.Error{Code: weibo.CodeInternal, Message: "v2ex provider is not configured"}
	}
	section = strings.ToLower(strings.TrimSpace(section))
	if section == "" {
		section = "latest"
	}
	if section != "latest" && section != "hot" {
		return nil, &weibo.Error{Code: weibo.CodeBadRequest, Message: fmt.Sprintf("unknown v2ex section %q", section)}
	}
	if limit <= 0 {
		limit = DefaultV2exLimit
	}
	if limit > MaxV2exLimit {
		limit = MaxV2exLimit
	}

	// Same cache-shape rule as sspai: the entry holds the list upstream gave
	// us and is widened on demand, never sized by whichever limit arrived
	// first. One V2EX request returns the whole list anyway (index.xml carries
	// 50 entries, the hot tab is a single page).
	items, hit, err := fetchCachedList(ctx, s, "v2ex:"+section, v2exTTL, refresh, limit,
		func(n int) ([]v2ex.Article, error) {
			if section == "hot" {
				return s.clients.V2ex.Hot(ctx, n)
			}
			return s.clients.V2ex.Latest(ctx, n)
		})
	if err != nil {
		return nil, sspaiError(err)
	}
	if len(items) > limit {
		items = items[:limit]
	}

	now := s.now().UTC()
	f := &Feed{
		FetchedAt: now,
		Cached:    hit,
		TTL:       15,
		Items:     make([]Item, 0, len(items)),
	}
	if section == "hot" {
		f.TitleText = "V2EX 最热"
		f.Description = "V2EX 最热主题（按回复数排序）"
	} else {
		f.TitleText = "V2EX 最新"
		f.Description = "V2EX 最新主题"
	}
	f.LinkText = "https://www.v2ex.com"

	for _, a := range items {
		created := a.Published
		if created.IsZero() {
			created = now
		}
		images := []string{}
		if m := v2exImg.FindStringSubmatch(a.Content); m != nil {
			images = append(images, m[1])
		}
		f.Items = append(f.Items, Item{
			ID:        strconv.Itoa(a.ID),
			Title:     a.Title,
			Text:      HTMLToText(a.Content),
			HTML:      v2exHTML(a, section),
			URL:       a.URL,
			CreatedAt: created.UTC(),
			Images:    images,
		})
	}
	return f, nil
}

func v2exHTML(a v2ex.Article, section string) string {
	var b strings.Builder
	meta := []string{}
	if a.Author != "" {
		meta = append(meta, "@"+a.Author)
	}
	if a.Node != "" {
		meta = append(meta, "#"+strings.ReplaceAll(a.Node, " ", ""))
	}
	if section == "hot" {
		meta = append(meta, fmt.Sprintf("%d 条回复", a.Replies))
	}
	if len(meta) > 0 {
		b.WriteString("<p>")
		b.WriteString(EscapeXML(strings.Join(meta, " · ")))
		b.WriteString("</p>")
	}
	if strings.TrimSpace(a.Content) != "" {
		b.WriteString(NormalizeHTML(a.Content))
	} else {
		b.WriteString(`<p><a href="`)
		b.WriteString(EscapeXML(a.URL))
		b.WriteString(`">查看主题</a></p>`)
	}
	return b.String()
}
