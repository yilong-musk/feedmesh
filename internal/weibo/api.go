package weibo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	uidPattern    = regexp.MustCompile(`^[0-9]{1,20}$`)
	domainPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	uidInURL      = regexp.MustCompile(`/u/([0-9]{1,20})`)
)

// ValidateUID rejects anything that is not a plain decimal weibo user id.
func ValidateUID(uid string) error {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return &Error{Code: CodeBadRequest, Message: "uid is required"}
	}
	if !uidPattern.MatchString(uid) {
		return &Error{Code: CodeBadRequest, Message: fmt.Sprintf("invalid uid %q: expected decimal digits", uid)}
	}
	return nil
}

// ValidateDomain rejects vanity names that could escape the URL path.
func ValidateDomain(domain string) error {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return &Error{Code: CodeBadRequest, Message: "domain is required"}
	}
	if !domainPattern.MatchString(domain) {
		return &Error{Code: CodeBadRequest,
			Message: fmt.Sprintf("invalid domain %q: expected letters, digits, dash or underscore", domain)}
	}
	return nil
}

// Profile resolves a user id to its account metadata.
func (c *Client) Profile(ctx context.Context, uid string) (*Profile, error) {
	if err := ValidateUID(uid); err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/api/container/getIndex?type=uid&value=%s", c.base(), url.QueryEscape(uid))
	var ir indexResponse
	status, err := c.fetchJSON(ctx, endpoint, c.base()+"/u/"+uid, &ir)
	if err != nil {
		return nil, err
	}
	if ir.OK != 1 {
		return nil, &Error{Code: CodeUserNotFound, Status: status,
			Message: fmt.Sprintf("weibo user %s not found: %s", uid, strings.TrimSpace(ir.Msg))}
	}
	if ir.Data.UserInfo == nil {
		return nil, &Error{Code: CodeUserNotFound, Status: status,
			Message: fmt.Sprintf("weibo user %s not found", uid)}
	}
	ui := ir.Data.UserInfo
	if strings.TrimSpace(ui.ScreenName) == "" {
		return nil, &Error{Code: CodeLoginRequired, Status: status,
			Message: fmt.Sprintf("weibo user %s has no public profile (login required or empty account)", uid)}
	}

	p := &Profile{
		UID:         uid,
		ScreenName:  ui.ScreenName,
		Description: ui.Description,
		AvatarURL:   ui.AvatarURL,
		Followers:   parseCount(ui.Followers),
		Statuses:    parseCount(ui.Statuses),
	}
	p.ContainerID = "107603" + uid
	if ir.Data.TabsInfo != nil {
		for _, t := range ir.Data.TabsInfo.Tabs {
			if t.Type == "weibo" && t.ContainerID != "" {
				p.ContainerID = t.ContainerID
				break
			}
		}
	}
	return p, nil
}

// Timeline fetches one page of a user's posts. limit <= 0 keeps the whole page.
func (c *Client) Timeline(ctx context.Context, uid, containerID string, limit int) (*Timeline, error) {
	if err := ValidateUID(uid); err != nil {
		return nil, err
	}
	if containerID == "" {
		containerID = "107603" + uid
	}
	endpoint := fmt.Sprintf("%s/api/container/getIndex?type=uid&value=%s&containerid=%s",
		c.base(), url.QueryEscape(uid), url.QueryEscape(containerID))

	var ir indexResponse
	status, err := c.fetchJSON(ctx, endpoint, c.base()+"/u/"+uid, &ir)
	if err != nil {
		return nil, err
	}
	if ir.OK != 1 {
		return nil, &Error{Code: CodeUserNotFound, Status: status,
			Message: fmt.Sprintf("weibo timeline %s unavailable: %s", uid, strings.TrimSpace(ir.Msg))}
	}

	tl := &Timeline{UID: uid, ContainerID: containerID}
	for _, card := range ir.Data.Cards {
		if card.MBlog == nil {
			continue
		}
		tl.Items = append(tl.Items, card.MBlog)
	}
	if limit > 0 && len(tl.Items) > limit {
		tl.Items = tl.Items[:limit]
	}
	return tl, nil
}

// ResolveDomain maps a vanity name (e.g. "rmrb") to its numeric uid.
func (c *Client) ResolveDomain(ctx context.Context, domain string) (string, error) {
	if err := ValidateDomain(domain); err != nil {
		return "", err
	}
	endpoint := c.base() + "/" + url.PathEscape(domain) + "?&jumpfrom=weibocom"
	body, status, finalURL, err := c.do(ctx, endpoint, c.base()+"/", modeDocument)
	if status == http.StatusNotFound {
		return "", &Error{Code: CodeUserNotFound, Status: status,
			Message: fmt.Sprintf("weibo domain %q not found", domain)}
	}
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", &Error{Code: CodeUpstreamUnavailable, Status: status,
			Message: fmt.Sprintf("domain page returned HTTP %d", status)}
	}
	if m := uidInURL.FindStringSubmatch(finalURL); len(m) == 2 {
		return m[1], nil
	}
	// The page may render the redirect client-side; look for a /u/<uid> link.
	if m := uidInURL.FindSubmatch(body); len(m) == 2 {
		return string(m[1]), nil
	}
	return "", &Error{Code: CodeUserNotFound,
		Message: fmt.Sprintf("could not resolve weibo domain %q to a uid", domain)}
}

// LongText upgrades a truncated long post via statuses/extend.
func (c *Client) LongText(ctx context.Context, id string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", &Error{Code: CodeBadRequest, Message: "status id is required"}
	}
	endpoint := fmt.Sprintf("%s/statuses/extend?id=%s", c.base(), url.QueryEscape(id))
	var er extendResponse
	status, err := c.fetchJSON(ctx, endpoint, c.base()+"/detail/"+id, &er)
	if err != nil {
		return "", err
	}
	if er.OK != 1 || er.Data == nil || strings.TrimSpace(er.Data.LongTextContent) == "" {
		return "", &Error{Code: CodeUpstreamUnavailable, Status: status,
			Message: fmt.Sprintf("statuses/extend returned no long text for %s", id)}
	}
	return er.Data.LongTextContent, nil
}

// StatusText is the fallback path when statuses/extend fails.
func (c *Client) StatusText(ctx context.Context, id string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", &Error{Code: CodeBadRequest, Message: "status id is required"}
	}
	endpoint := fmt.Sprintf("%s/statuses/show?id=%s", c.base(), url.QueryEscape(id))
	var sr showResponse
	status, err := c.fetchJSON(ctx, endpoint, c.base()+"/detail/"+id, &sr)
	if err != nil {
		return "", err
	}
	if sr.OK != 1 || sr.Data == nil || strings.TrimSpace(sr.Data.Text) == "" {
		return "", &Error{Code: CodeUpstreamUnavailable, Status: status,
			Message: fmt.Sprintf("statuses/show returned no text for %s", id)}
	}
	return sr.Data.Text, nil
}

// Step is one doctor probe result.
type Step struct {
	Step       string `json:"step"`
	OK         bool   `json:"ok"`
	MS         int64  `json:"ms"`
	HTTPStatus int    `json:"http_status"`
	Code       string `json:"code,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Probe exercises the whole upstream chain: visitor cookie, getIndex, statuses.
func (c *Client) Probe(ctx context.Context, uid string) []Step {
	if uid == "" {
		uid = "2803301701"
	}
	steps := make([]Step, 0, 3)

	start := time.Now()
	_, err := c.visitor(ctx, true)
	status := 200
	if err != nil {
		status = StatusOf(err)
	}
	steps = append(steps, stepResult("genvisitor", start, status, err))

	start = time.Now()
	profile, err := c.Profile(ctx, uid)
	status = 200
	if err != nil {
		status = StatusOf(err)
	}
	steps = append(steps, stepResult("getIndex", start, status, err))

	start = time.Now()
	var stepErr error
	stepStatus := 200
	if err != nil {
		stepErr = fmt.Errorf("skipped: getIndex failed")
		stepStatus = 0
	} else {
		tl, tlErr := c.Timeline(ctx, uid, profile.ContainerID, 1)
		if tlErr != nil {
			stepErr = tlErr
			stepStatus = StatusOf(tlErr)
		} else if len(tl.Items) == 0 {
			stepErr = fmt.Errorf("no posts returned for uid %s", uid)
			stepStatus = 0
		} else if text, textErr := c.StatusText(ctx, tl.Items[0].numericID()); textErr != nil {
			stepErr = textErr
			stepStatus = StatusOf(textErr)
		} else if strings.TrimSpace(text) == "" {
			stepErr = fmt.Errorf("empty status body")
		}
	}
	steps = append(steps, stepResult("statuses", start, stepStatus, stepErr))
	return steps
}

func stepResult(name string, start time.Time, status int, err error) Step {
	s := Step{Step: name, OK: err == nil, MS: time.Since(start).Milliseconds(), HTTPStatus: status}
	if err != nil {
		s.Error = err.Error()
		s.Code = CodeOf(err)
		if status == 0 {
			s.HTTPStatus = StatusOf(err)
		}
	}
	return s
}

// ParseMBlogID is exported for tests and callers that only need the id string.
func ParseMBlogID(raw string) string {
	var f flexString
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		return ""
	}
	return f.String()
}
