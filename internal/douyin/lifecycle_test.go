package douyin

import (
	"context"
	"errors"
	"testing"
	"time"
)

const testSecUID = "MS4wLjABAAAAlVAfjJp4JQqjNIFKLDwWDHjjcS11yIMJ2a1kRhLiAD8"

// hideChrome makes sure no Chromium can be found, so a test never starts a
// real browser.
func hideChrome(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FEEDMESH_CHROME", "")
}

func TestBrowserReplacesADeadBrowser(t *testing.T) {
	hideChrome(t)
	c := New(Config{ChromePath: "/nonexistent/chrome"})
	dead := &browser{}
	dead.dead.Store(true)
	c.br = dead

	if _, err := c.browser(); err == nil {
		t.Fatal("expected a start error with no Chromium installed")
	}
	if c.br == dead {
		t.Fatal("a dead browser was handed out again instead of being replaced")
	}
}

func TestBrowserKeepsALiveBrowser(t *testing.T) {
	c := New(Config{})
	live := &browser{}
	c.br = live
	got, err := c.browser()
	if err != nil || got != live {
		t.Fatalf("browser() = %p, %v; want the live browser %p", got, err, live)
	}
}

func TestTimelineQueuedCallerLeavesOnItsDeadline(t *testing.T) {
	hideChrome(t)
	c := New(Config{Timeout: time.Minute})
	c.slot <- struct{}{} // another Timeline call owns the page
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	begin := time.Now()
	_, err := c.Timeline(ctx, testSecUID, 5)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the caller's deadline", err)
	}
	if waited := time.Since(begin); waited > time.Second {
		t.Fatalf("queued caller waited %s", waited)
	}
}
