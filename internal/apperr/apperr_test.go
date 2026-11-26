package apperr

import (
	"errors"
	"fmt"
	"testing"
)

func TestCodeAndStatusSurviveWrapping(t *testing.T) {
	err := fmt.Errorf("outer: %w", New(CodeUpstreamBlocked, 418, "blocked %s", "x"))
	if CodeOf(err) != CodeUpstreamBlocked || StatusOf(err) != 418 {
		t.Fatalf("CodeOf/StatusOf = %q/%d", CodeOf(err), StatusOf(err))
	}
	if CodeOf(errors.New("plain")) != CodeInternal || StatusOf(errors.New("plain")) != 0 {
		t.Fatal("an unclassified error must read as INTERNAL with no status")
	}
	cause := errors.New("cause")
	if e := (&Error{Code: CodeNetwork, Err: cause}); !errors.Is(e, cause) || e.Error() != "cause" {
		t.Fatalf("Error/Unwrap = %q", e.Error())
	}
}
