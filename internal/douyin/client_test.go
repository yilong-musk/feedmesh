package douyin

import (
	"testing"
	"time"
)

// upstreamPage mirrors the fields the client reads from one API reply. Two
// videos are enough: one pinned older item listed first, one newer item.
const upstreamPage = `{
  "status_code": 0,
  "has_more": 1,
  "aweme_list": [
    {
      "aweme_id": "7000000000000000001",
      "desc": "pinned older video",
      "create_time": 1700000000,
      "is_top": 1,
      "author": {"nickname": "Test Creator", "sec_uid": "MS4wLjABAAAAtestsecuid0000000000"},
      "video": {
        "duration": 93000,
        "width": 1080,
        "height": 1920,
        "play_addr": {"url_list": ["https://v3-dy-o.zjcdn.com/a.mp4"]},
        "cover": {"url_list": ["https://p3.douyinpic.com/cover.jpeg"]},
        "poster": ""
      },
      "images": []
    },
    {
      "aweme_id": "7000000000000000002",
      "desc": "newest video #tag",
      "create_time": 1800000000,
      "is_top": 0,
      "author": {"nickname": "Test Creator", "sec_uid": "MS4wLjABAAAAtestsecuid0000000000"},
      "video": {
        "duration": 15000,
        "width": 1080,
        "height": 1920,
        "play_addr": {"url_list": []},
        "cover": {"url_list": ["https://p3.douyinpic.com/cover2.jpeg"]},
        "poster": "https://p3.douyinpic.com/poster2.jpeg"
      },
      "images": []
    }
  ]
}`

func TestParseTimelineSortsByPublishTime(t *testing.T) {
	tl, err := parseTimeline(upstreamPage, "MS4wLjABAAAAtestsecuid0000000000", 10)
	if err != nil {
		t.Fatal(err)
	}
	if tl.Author != "Test Creator" {
		t.Fatalf("author = %q", tl.Author)
	}
	if len(tl.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(tl.Items))
	}
	if tl.Items[0].ID != "7000000000000000002" {
		t.Fatalf("first item = %s, want the newest one", tl.Items[0].ID)
	}
	if tl.Items[0].CreatedAt != time.Unix(1800000000, 0).UTC() {
		t.Fatalf("created_at = %v", tl.Items[0].CreatedAt)
	}
	if !tl.Items[1].IsTop {
		t.Fatal("pinned marker lost")
	}
	if tl.Items[1].PlayURL == "" {
		t.Fatal("play url missing")
	}
	if tl.Items[0].CoverURL != "https://p3.douyinpic.com/poster2.jpeg" {
		t.Fatalf("cover = %q, want the poster", tl.Items[0].CoverURL)
	}
	if tl.Items[0].DurationMS != 15000 || tl.Items[0].Width != 1080 {
		t.Fatalf("media fields = %+v", tl.Items[0])
	}
}

func TestParseTimelineTrimsToLimit(t *testing.T) {
	tl, err := parseTimeline(upstreamPage, "MS4wLjABAAAAtestsecuid0000000000", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(tl.Items) != 1 || tl.Items[0].ID != "7000000000000000002" {
		t.Fatalf("items = %+v", tl.Items)
	}
}

func TestValidSecUID(t *testing.T) {
	valid := "MS4wLjABAAAAlVAfjJp4JQqjNIFKLDwWDHjjcS11yIMJ2a1kRhLiAD8"
	if !validSecUID(valid) {
		t.Fatal("expected a valid sec_uid")
	}
	for _, bad := range []string{"", "short", "has space in it 000000000000", "sl/ash00000000000000000000"} {
		if validSecUID(bad) {
			t.Fatalf("%q should be rejected", bad)
		}
	}
}
