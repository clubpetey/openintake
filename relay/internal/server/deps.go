package server

import "intake/internal/version"

// Deps holds the dependencies injected into the HTTP server at startup.
//
// 1-i owns: Version, CORSOrigins.
// Extended by 1-iii: Auth (session middleware), Provider (llm.Provider),
//   SystemPrompt (string), Model (string), MaxTokens (int).
// Extended by 1-iv: Adapter (adapter.Adapter), Classifier, Builder.
//
// Later sub-plans assign their fields after constructing the relay server.
// The struct is defined here in full so all sub-plans share one type without
// circular imports — packages that are not yet created are NOT imported here;
// their fields will be added to this struct by those sub-plans as interface{}
// or concrete types once the packages exist.
type Deps struct {
	// Version is populated from the binary's build-time ldflags.
	Version version.BuildInfo

	// CORSOrigins is the strict allowlist of origins that may make cross-origin
	// requests. Populated from cfg.Server.CORSOrigins in main.go.
	CORSOrigins []string

	// extended by 1-iii (Auth, Provider, SystemPrompt, Model, MaxTokens)
	// and 1-iv (Adapter, Classifier, Builder)
}
