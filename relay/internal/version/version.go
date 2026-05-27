package version

// Build-time variables. Override with:
//
//	go build -ldflags "-X intake/internal/version.version=v1.2.3 \
//	  -X intake/internal/version.commit=abc1234 \
//	  -X intake/internal/version.buildTime=2026-01-01T00:00:00Z" ./cmd/relay
var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

// BuildInfo carries the binary's identity, populated at link time via -ldflags.
type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

// Info returns the build information for this binary.
func Info() BuildInfo {
	return BuildInfo{
		Version:   version,
		Commit:    commit,
		BuildTime: buildTime,
	}
}
