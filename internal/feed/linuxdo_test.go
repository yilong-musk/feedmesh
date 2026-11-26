package feed

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/apperr"
	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/linuxdo"
	"github.com/yilong-musk/feedmesh/internal/logx"
)

func linuxdoTestFeed() *linuxdo.Feed {
	at := time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC)
	return &linuxdo.Feed{
		Kind:      linuxdo.KindLatest,
		Channel:   "LINUX DO - 最新话题",
		Link:      "https://linux.do/latest",
		Language:  "zh-CN",
		FetchedAt: at,
		Topics: []linuxdo.Topic{
			{
				ID: "3191226", Title: "第一帖", URL: "https://linux.do/t/topic/3191226",
				Author: "alice", Category: "开发调优", CreatedAt: at.Add(2 * time.Minute),
				HTML:   `<p>第一段</p><p>第二段</p><p><img src="https://cdn3.ldstatic.com/a.jpg"></p>`,
				Images: []string{"https://cdn3.ldstatic.com/a.jpg"},
			},
			{
				ID: "3191225", Title: "第二帖", URL: "https://linux.do/t/topic/3191225",
				Author: "bob", Category: "搞七捻三", CreatedAt: at.Add(time.Minute),
				Pinned: true, Closed: true,
				HTML: `<p>正文</p>`,
			},
			{
				ID: "3191224", Title: "第三帖", URL: "https://linux.do/t/topic/3191224",
				Author: "carol", Category: "开发调优", CreatedAt: at,
				HTML: `<p>正文</p>`,
			},
		},
	}
}

type stubLinuxDo struct {
	feed  *linuxdo.Feed
	err   error
	calls []int
}

func (s *stubLinuxDo) Fetch(_ context.Context, _ linuxdo.Ref, limit int) (*linuxdo.Feed, error) {
	s.calls = append(s.calls, limit)
	if s.err != nil {
		return nil, s.err
	}
	out := *s.feed
	out.Topics = append([]linuxdo.Topic(nil), s.feed.Topics...)
	if limit > 0 && limit < len(out.Topics) {
		out.Topics = out.Topics[:limit]
	}
	return &out, nil
}

func (s *stubLinuxDo) Budget() time.Duration { return 5 * time.Second }

func TestGetLinuxDoBuildsFeed(t *testing.T) {
	svc := NewService(Clients{LinuxDo: &stubLinuxDo{feed: linuxdoTestFeed()}}, nil, logx.Discard(), DefaultTTLs())
	f, err := svc.GetLinuxDo(context.Background(), linuxdo.Ref{Kind: linuxdo.KindLatest}, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if f.Title() != "LINUX DO · Latest topics" {
		t.Fatalf("title = %q", f.Title())
	}
	if f.Link() != "https://linux.do/latest" {
		t.Fatalf("link = %q", f.Link())
	}
	if len(f.Items) != 3 {
		t.Fatalf("items = %d", len(f.Items))
	}
	first := f.Items[0]
	if first.ID != "3191226" || first.URL != "https://linux.do/t/topic/3191226" {
		t.Fatalf("first = %+v", first)
	}
	if first.CreatedAt.Location() != time.UTC {
		t.Fatalf("created_at is not UTC: %s", first.CreatedAt)
	}
	if first.Text != "第一段\n第二段" {
		t.Fatalf("text = %q", first.Text)
	}
	if !strings.Contains(first.HTML, "Category 开发调优") || !strings.Contains(first.HTML, "by @alice") {
		t.Fatalf("meta missing: %s", first.HTML)
	}
	if len(first.Images) != 1 || first.Images[0] != "https://cdn3.ldstatic.com/a.jpg" {
		t.Fatalf("images = %v", first.Images)
	}
	second := f.Items[1]
	if !strings.Contains(second.HTML, "Pinned") || !strings.Contains(second.HTML, "Closed") {
		t.Fatalf("state labels missing: %s", second.HTML)
	}
}

func TestGetLinuxDoTitles(t *testing.T) {
	svc := NewService(Clients{LinuxDo: &stubLinuxDo{feed: linuxdoTestFeed()}}, nil, logx.Discard(), DefaultTTLs())
	cases := []struct {
		ref   linuxdo.Ref
		title string
		link  string
	}{
		{linuxdo.Ref{Kind: linuxdo.KindTop, Period: "weekly"}, "LINUX DO · Top Weekly", "https://linux.do/top?period=weekly"},
		{linuxdo.Ref{Kind: linuxdo.KindHot}, "LINUX DO · Hot", "https://linux.do/hot"},
		{linuxdo.Ref{Kind: linuxdo.KindCategory, Slug: "develop", ID: "4"}, "LINUX DO · 开发调优", "https://linux.do/c/develop/4"},
		{linuxdo.Ref{Kind: linuxdo.KindTag, Tag: "chatgpt"}, "LINUX DO · #chatgpt", "https://linux.do/tag/chatgpt"},
	}
	for _, tc := range cases {
		f, err := svc.GetLinuxDo(context.Background(), tc.ref, 10, false)
		if err != nil {
			t.Fatalf("%+v: %v", tc.ref, err)
		}
		if f.Title() != tc.title || f.Link() != tc.link {
			t.Errorf("%+v title/link = %q / %q, want %q / %q", tc.ref, f.Title(), f.Link(), tc.title, tc.link)
		}
	}
}

func TestGetLinuxDoLimitAndCacheWiden(t *testing.T) {
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stub := &stubLinuxDo{feed: linuxdoTestFeed()}
	svc := NewService(Clients{LinuxDo: stub}, c, logx.Discard(), DefaultTTLs())
	ctx := context.Background()
	narrow, err := svc.GetLinuxDo(ctx, linuxdo.Ref{Kind: linuxdo.KindLatest}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(narrow.Items) != 1 {
		t.Fatalf("narrow items = %d", len(narrow.Items))
	}
	wide, err := svc.GetLinuxDo(ctx, linuxdo.Ref{Kind: linuxdo.KindLatest}, 3, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(wide.Items) != 3 {
		t.Fatalf("wide items = %d", len(wide.Items))
	}
	again, err := svc.GetLinuxDo(ctx, linuxdo.Ref{Kind: linuxdo.KindLatest}, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Cached || len(again.Items) != 2 {
		t.Fatalf("cache read = cached %v items %d", again.Cached, len(again.Items))
	}
	if len(stub.calls) != 2 || stub.calls[0] != 1 || stub.calls[1] != 3 {
		t.Fatalf("upstream widths = %v, want [1 3]", stub.calls)
	}
}

func TestGetLinuxDoErrors(t *testing.T) {
	cases := []struct {
		name string
		ref  linuxdo.Ref
		err  error
		want string
	}{
		{"invalid", linuxdo.Ref{Kind: "nope"}, nil, apperr.CodeBadRequest},
		{"missing", linuxdo.Ref{Kind: linuxdo.KindLatest}, linuxdo.ErrNotFound, apperr.CodeUserNotFound},
		{"challenge", linuxdo.Ref{Kind: linuxdo.KindLatest}, linuxdo.ErrChallenge, apperr.CodeUpstreamBlocked},
		{"empty", linuxdo.Ref{Kind: linuxdo.KindLatest}, linuxdo.ErrNoItems, apperr.CodeUpstreamUnavailable},
		{"garbage", linuxdo.Ref{Kind: linuxdo.KindLatest}, linuxdo.ErrUnexpectedPage, apperr.CodeUpstreamUnavailable},
		{"timeout", linuxdo.Ref{Kind: linuxdo.KindLatest}, context.DeadlineExceeded, apperr.CodeTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(Clients{LinuxDo: &stubLinuxDo{err: tc.err}}, nil, logx.Discard(), DefaultTTLs())
			_, err := svc.GetLinuxDo(context.Background(), tc.ref, 10, false)
			if err == nil {
				t.Fatal("expected an error")
			}
			if code := apperr.CodeOf(err); code != tc.want {
				t.Fatalf("code = %s, want %s (err %v)", code, tc.want, err)
			}
		})
	}
}

func TestLinuxDoStaleWindow(t *testing.T) {
	if got := staleWindowFor("linuxdo:feed:latest:created", LinuxDoTTL()); got != 24*time.Hour {
		t.Fatalf("stale window = %s, want 24h", got)
	}
}

func TestLinuxDoRefreshKeepsWideCache(t *testing.T) {
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stub := &stubLinuxDo{feed: linuxdoTestFeed()}
	svc := NewService(Clients{LinuxDo: stub}, c, logx.Discard(), DefaultTTLs())
	ctx := context.Background()
	if _, err := svc.GetLinuxDo(ctx, linuxdo.Ref{Kind: linuxdo.KindLatest}, 90, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetLinuxDo(ctx, linuxdo.Ref{Kind: linuxdo.KindLatest}, 30, true); err != nil {
		t.Fatal(err)
	}
	if len(stub.calls) != 2 || stub.calls[0] != 90 || stub.calls[1] != 90 {
		t.Fatalf("refresh widths = %v, want [90 90] (a narrow refresh must keep the wide cache)", stub.calls)
	}
}
