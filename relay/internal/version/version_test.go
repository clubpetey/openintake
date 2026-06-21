package version_test

import (
	"testing"

	"github.com/clubpetey/openintake/relay/internal/version"
)

func TestBuildInfo_Defaults(t *testing.T) {
	info := version.Info()
	if info.Version == "" {
		t.Error("Version is empty; want non-empty default")
	}
	if info.Commit == "" {
		t.Error("Commit is empty; want non-empty default")
	}
	if info.BuildTime == "" {
		t.Error("BuildTime is empty; want non-empty default")
	}
	// When not overridden via ldflags the defaults are "dev", "none", "unknown".
	if info.Version != "dev" {
		t.Errorf("default Version = %q; want %q", info.Version, "dev")
	}
}
