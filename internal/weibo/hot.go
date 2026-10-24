package weibo

import (
	"context"
	"fmt"
	"strings"
)

// hotSearchURL is the desktop web endpoint that serves the realtime hot list.
const hotSearchURL = "https://weibo.com/ajax/side/hotSearch"

// HotItem is one entry of the realtime hot search list.
type HotItem struct {
	Rank       int    `json:"rank"`
	Word       string `json:"word"`
	WordScheme string `json:"word_scheme"`
	Note       string `json:"note"`
	Label      string `json:"label_name"`
	Category   string `json:"flag_desc"`
	Heat       int64  `json:"num"`
	TopicFlag  int    `json:"topic_flag"`
	RealPos    int    `json:"realpos"`
}

type hotResponse struct {
	OK   int `json:"ok"`
	Data struct {
		Realtime []HotItem `json:"realtime"`
	} `json:"data"`
}

// HotSearch returns the realtime hot search list, ranked from 1.
//
// The endpoint lives on weibo.com (not m.weibo.cn) and requires the visitor
// cookie plus a weibo.com referer; both are supplied by the shared client.
func (c *Client) HotSearch(ctx context.Context) ([]HotItem, error) {
	var hr hotResponse
	status, err := c.fetchJSON(ctx, c.cfg.HotURL, "https://weibo.com/", &hr)
	if err != nil {
		return nil, err
	}
	if hr.OK != 1 {
		return nil, &Error{Code: CodeUpstreamUnavailable, Status: status,
			Message: fmt.Sprintf("hot search endpoint returned ok=%d", hr.OK)}
	}
	items := make([]HotItem, 0, len(hr.Data.Realtime))
	for _, it := range hr.Data.Realtime {
		it.Word = strings.TrimSpace(it.Word)
		// Entries without realpos are promoted/recommended slots, not part of
		// the ranked list; keep them out so ranks stay 1..50.
		if it.Word == "" || it.RealPos <= 0 {
			continue
		}
		it.WordScheme = strings.TrimSpace(strings.Trim(it.WordScheme, "#"))
		if it.WordScheme == "" {
			it.WordScheme = it.Word
		}
		it.Rank = it.RealPos
		items = append(items, it)
	}
	if len(items) == 0 {
		return nil, &Error{Code: CodeUpstreamUnavailable, Status: status,
			Message: "hot search endpoint returned an empty list"}
	}
	return items, nil
}
