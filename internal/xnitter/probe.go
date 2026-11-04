package xnitter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Step is one doctor line. The JSON shape matches the Weibo probe so the
// `feedmesh doctor --json` contract stays the same across providers; Items is
// the only extra field.
type Step struct {
	Step       string `json:"step"`
	OK         bool   `json:"ok"`
	MS         int64  `json:"ms"`
	HTTPStatus int    `json:"http_status"`
	Code       string `json:"code,omitempty"`
	Error      string `json:"error,omitempty"`
	Items      int    `json:"items,omitempty"`
}

const (
	// probeInstances bounds how many mirrors one doctor run dials.
	probeInstances = 5
	// probeTimeout bounds one instance read.
	probeTimeout = 20 * time.Second
)

// Pool returns the current candidate list for diagnostics: the directory pool
// when it is reachable, the operator fallbacks otherwise. The error is
// informational; a fallback list is still returned with it.
func (c *Client) Pool(ctx context.Context) ([]string, error) {
	poolCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pool, err := c.fetchPool(poolCtx)
	if err == nil && len(pool) > 0 {
		return pool, nil
	}
	if err != nil {
		return c.fallbackWithPool(), err
	}
	return c.fallbackWithPool(), fmt.Errorf("directory returned no usable instances")
}

// Probe reports the pool and one line per instance for a handle. It is
// read-only: a 404 or 403 here never marks an instance as unhealthy, because a
// missing account is not a broken mirror.
func (c *Client) Probe(ctx context.Context, handle string) []Step {
	steps := make([]Step, 0, probeInstances+1)
	start := time.Now()
	pool, err := c.Pool(ctx)
	poolStep := Step{Step: "pool", MS: time.Since(start).Milliseconds()}
	// The directory is aggressively rate-limited, so a fallback list is a
	// success for diagnostics: the step only fails when there is nothing to
	// dial at all. The error text stays as an operator note.
	if err != nil {
		poolStep.Error = err.Error()
	}
	if len(pool) > 0 {
		poolStep.OK = true
		poolStep.Items = len(pool)
	}
	steps = append(steps, poolStep)

	h, err := NormalizeHandle(handle)
	if err != nil {
		steps = append(steps, Step{Step: "handle", OK: false, Code: "BAD_REQUEST", Error: err.Error()})
		return steps
	}
	if len(pool) > probeInstances {
		pool = pool[:probeInstances]
	}
	for _, base := range pool {
		if ctx.Err() != nil {
			break
		}
		steps = append(steps, c.probeInstance(ctx, base, h))
	}
	return steps
}

func (c *Client) probeInstance(ctx context.Context, base, handle string) Step {
	step := Step{Step: hostOf(base)}
	target := strings.TrimRight(base, "/") + "/" + handle + "/rss"
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		step.Error = err.Error()
		return step
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "application/rss+xml, application/xml;q=0.9, */*;q=0.8")
	start := time.Now()
	resp, err := c.http.Do(req)
	step.MS = time.Since(start).Milliseconds()
	if err != nil {
		step.Code = "TIMEOUT"
		step.Error = err.Error()
		return step
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	step.HTTPStatus = resp.StatusCode
	step.Items = bytes.Count(body, []byte("<item>"))
	switch {
	case resp.StatusCode == http.StatusOK && step.Items > 0:
		step.OK = true
	case resp.StatusCode == http.StatusOK:
		step.Error = "200 but no items"
	case resp.StatusCode == http.StatusTooManyRequests:
		step.Code = "RATE_LIMITED"
		step.Error = "HTTP 429"
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnavailableForLegalReasons:
		step.Code = "UPSTREAM_BLOCKED"
		step.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
	default:
		step.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return step
}
