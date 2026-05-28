// Package budget tracks daily LLM token spend for the relay. Phase 5-i
// exports the empty type as a placeholder so the server chain compiles;
// 5-ii fills in the actual Reserve/Commit implementation.
package budget

import "time"

// Tracker is the daily-budget tracker. 5-i placeholder; 5-ii implements.
type Tracker struct {
	// 5-ii populates with {in, out, max, dayStartUTC, now, mu}.
}

// New constructs a Tracker. 5-i placeholder; 5-ii implements.
// 5-i callers in main.go pass nil to Deps.Budget; do NOT call this in 5-i.
func New(maxInputTokens, maxOutputTokens int, now func() time.Time) *Tracker {
	_ = maxInputTokens
	_ = maxOutputTokens
	_ = now
	return &Tracker{}
}

// Reserve / Commit / Snapshot: 5-ii implements. 5-i: stub methods that
// always allow (so a downstream caller's nil-check is the gate, not these).
func (t *Tracker) Reserve(tenantKey string, estIn, estOut int) (ok bool, retryAfter time.Duration) {
	_ = tenantKey
	_ = estIn
	_ = estOut
	return true, 0
}
func (t *Tracker) Commit(tenantKey string, actualIn, actualOut int) {
	_ = tenantKey
	_ = actualIn
	_ = actualOut
}
func (t *Tracker) Snapshot(tenantKey string) (in, out int, dayStartUTC time.Time) {
	_ = tenantKey
	return 0, 0, time.Time{}
}
