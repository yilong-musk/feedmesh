package weibo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// flexString accepts either a JSON string or a JSON number.
//
// weibo is inconsistent: mblog.id arrives quoted, while a few id-ish fields
// arrive bare. Decoding both shapes keeps the model honest.
type flexString string

// UnmarshalJSON implements json.Unmarshaler.
func (f *flexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexString(n.String())
	return nil
}

// String returns the raw value.
func (f flexString) String() string { return string(f) }

// User is the author block embedded in profiles and posts.
type User struct {
	ID          flexString `json:"id"`
	ScreenName  string     `json:"screen_name"`
	Description string     `json:"description"`
	AvatarURL   string     `json:"profile_image_url"`
	Followers   flexString `json:"followers_count"`
	Statuses    flexString `json:"statuses_count"`
}

// Geo is the pixel size weibo reports for an image rendition.
type Geo struct {
	Width  flexString `json:"width"`
	Height flexString `json:"height"`
}

// Pic is one image attached to a post.
type Pic struct {
	PID   string `json:"pid"`
	URL   string `json:"url"`
	Size  string `json:"size"`
	Geo   *Geo   `json:"geo"`
	Large *struct {
		URL  string `json:"url"`
		Size string `json:"size"`
		Geo  *Geo   `json:"geo"`
	} `json:"large"`
}

// BestURL prefers the largest rendition available.
func (p Pic) BestURL() string {
	if p.Large != nil && p.Large.URL != "" {
		return p.Large.URL
	}
	return p.URL
}

// Dimensions returns the pixel size of BestURL when weibo reports it. Either
// value may be empty; callers must tolerate that.
func (p Pic) Dimensions() (width, height string) {
	if p.Large != nil && p.Large.Geo != nil {
		width, height = string(p.Large.Geo.Width), string(p.Large.Geo.Height)
	}
	if (width == "" || height == "") && p.Geo != nil {
		width, height = string(p.Geo.Width), string(p.Geo.Height)
	}
	return sanitizeDimension(width), sanitizeDimension(height)
}

func sanitizeDimension(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 6 {
		return ""
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return s
}

// PagePic is the cover image of a link or video card.
type PagePic struct {
	URL    string     `json:"url"`
	Width  flexString `json:"width"`
	Height flexString `json:"height"`
}

// MediaInfo carries the playable streams of a video card.
type MediaInfo struct {
	StreamURL   string  `json:"stream_url"`
	StreamURLHD string  `json:"stream_url_hd"`
	Duration    float64 `json:"duration"`
}

// PageInfo is the link card, video card or topic card attached to a post.
type PageInfo struct {
	Type       string     `json:"type"`
	ObjectType flexString `json:"object_type"`
	PageTitle  string     `json:"page_title"`
	Title      string     `json:"title"`
	Content1   string     `json:"content1"`
	Content2   string     `json:"content2"`
	PageURL    string     `json:"page_url"`
	URLOri     string     `json:"url_ori"`
	PlayCount  string     `json:"play_count"`
	PagePic    *PagePic   `json:"page_pic"`
	MediaInfo  *MediaInfo `json:"media_info"`
}

// IsVideo reports whether the card is a video card with a playable stream.
func (p *PageInfo) IsVideo() bool {
	return p != nil && p.Type == "video" && p.BestVideoURL() != ""
}

// BestVideoURL prefers the HD stream, then the standard one.
func (p *PageInfo) BestVideoURL() string {
	if p == nil || p.MediaInfo == nil {
		return ""
	}
	if u := strings.TrimSpace(p.MediaInfo.StreamURLHD); u != "" {
		return u
	}
	return strings.TrimSpace(p.MediaInfo.StreamURL)
}

// Link returns the card's click-through target, falling back to the short URL.
func (p *PageInfo) Link() string {
	if p == nil {
		return ""
	}
	if u := strings.TrimSpace(p.PageURL); u != "" {
		return u
	}
	return strings.TrimSpace(p.URLOri)
}

// Heading returns the most specific title the card carries.
func (p *PageInfo) Heading() string {
	if p == nil {
		return ""
	}
	for _, s := range []string{p.PageTitle, p.Title} {
		if t := strings.TrimSpace(s); t != "" {
			return t
		}
	}
	return ""
}

// MBlog is a single weibo post.
type MBlog struct {
	ID            flexString `json:"id"`
	MID           flexString `json:"mid"`
	BID           string     `json:"bid"`
	Text          string     `json:"text"`
	CreatedAt     string     `json:"created_at"`
	Source        string     `json:"source"`
	IsLongText    bool       `json:"isLongText"`
	StatusTitle   string     `json:"status_title"`
	Pics          []Pic      `json:"pics"`
	PicIDs        []string   `json:"pic_ids"`
	ThumbnailPic  string     `json:"thumbnail_pic"`
	RepostsCount  int        `json:"reposts_count"`
	CommentsCount int        `json:"comments_count"`
	Attitudes     int        `json:"attitudes_count"`
	User          *User      `json:"user"`
	Retweeted     *MBlog     `json:"retweeted_status"`
	PageInfo      *PageInfo  `json:"page_info"`
}

// tab is one entry of tabsInfo.tabs.
type tab struct {
	Type        string `json:"tab_type"`
	Name        string `json:"name"`
	ContainerID string `json:"containerid"`
}

// card is one entry of data.cards.
type card struct {
	CardType int    `json:"card_type"`
	MBlog    *MBlog `json:"mblog"`
}

// indexResponse covers both getIndex call shapes (profile and container).
//
// Observed 2026-10-23: the status flag lives at the TOP level, not under data
// (data.ok is absent). A bad uid yields HTTP 200 with {"ok":0,...}.
type indexResponse struct {
	OK   int    `json:"ok"`
	Msg  string `json:"msg"`
	Data struct {
		OK       *int  `json:"ok"`
		UserInfo *User `json:"userInfo"`
		TabsInfo *struct {
			Tabs []tab `json:"tabs"`
		} `json:"tabsInfo"`
		Cards []card `json:"cards"`
	} `json:"data"`
}

// Profile is a resolved weibo account.
type Profile struct {
	UID         string `json:"uid"`
	ScreenName  string `json:"screen_name"`
	Description string `json:"description"`
	AvatarURL   string `json:"avatar_url"`
	Followers   int64  `json:"followers"`
	Statuses    int64  `json:"statuses"`
	ContainerID string `json:"container_id"`
}

// Timeline is one page of a user's posts.
type Timeline struct {
	UID         string   `json:"uid"`
	ContainerID string   `json:"container_id"`
	Items       []*MBlog `json:"items"`
}

type extendResponse struct {
	OK   int `json:"ok"`
	Data *struct {
		LongTextContent string `json:"longTextContent"`
	} `json:"data"`
}

type showResponse struct {
	OK   int    `json:"ok"`
	Msg  string `json:"msg"`
	Data *MBlog `json:"data"`
}

type visitorResponse struct {
	Retcode int    `json:"retcode"`
	Msg     string `json:"msg"`
	Data    *struct {
		Sub  string `json:"sub"`
		Subp string `json:"subp"`
	} `json:"data"`
}

// bmid returns the best identifier for building a permalink.
func (m *MBlog) bmid() string {
	if m == nil {
		return ""
	}
	if m.BID != "" {
		return m.BID
	}
	return m.ID.String()
}

// BIDString returns the base-62 status id used in weibo.com permalinks.
func (m *MBlog) BIDString() string { return m.bmid() }

// IDString returns the numeric status id, which statuses/extend expects.
func (m *MBlog) IDString() string { return m.numericID() }

// numericID returns the decimal id as a string.
func (m *MBlog) numericID() string {
	if m == nil {
		return ""
	}
	if id := m.ID.String(); id != "" {
		return id
	}
	return m.MID.String()
}

// parseCount converts weibo's stringly-typed counters.
func parseCount(s flexString) int64 {
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(string(s), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func (p *Profile) String() string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("Profile(%s/%s)", p.UID, p.ScreenName)
}
