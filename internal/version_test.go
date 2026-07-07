// SPDX-License-Identifier: MIT

package internal

import (
	"strings"
	"testing"
)

func TestVersionUsesInjectedValue(t *testing.T) {
	old := version
	version = "v1.2.3"
	defer func() { version = old }()

	if got := Version(); got != "v1.2.3" {
		t.Errorf("Version() = %q, want v1.2.3", got)
	}
}

// TestVersionFallsBackWhenNotInjected can't control what `go test` stamps
// into debug.BuildInfo, but every fallback branch (module version, "dev-"+
// revision, or bare "dev") produces a non-empty string, so that's the
// invariant worth asserting.
func TestVersionFallsBackWhenNotInjected(t *testing.T) {
	old := version
	version = ""
	defer func() { version = old }()

	got := Version()
	if got == "" {
		t.Fatal("Version() returned empty string")
	}
	if strings.HasPrefix(got, "dev-") && len(strings.TrimPrefix(got, "dev-")) == 0 {
		t.Errorf("Version() = %q, want a revision after dev- prefix", got)
	}
}
