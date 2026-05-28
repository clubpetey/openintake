// Package budget tracks daily LLM token spend for the relay. Counters are keyed
// by tenantKey ("" when no X-Intake-Tenant header is present) and reset at
// 00:00 UTC (per-tenant; tenants roll over independently).
//
// Semantics: Reserve checks against current totals + estimate WITHOUT mutating;
// Commit records actual usage AFTER SSEDone. Reserve uses conservative
// caller-supplied estimates (4-chars-per-token input + cfg.MaxTokens for output)
// so the gate trips BEFORE an LLM credit is spent.
//
// L014: injectable clock; eager UTC-day reset on every Reserve/Commit access
// (no background goroutine).
package budget

import (
	"math"
	"sync"
	"time"
)

// dailyCounters holds one tenant's input/output token totals for the current
// UTC day. dayStartUTC is the truncated-to-day start of the active day.
type dailyCounters struct {
	in          int
	out         int
	dayStartUTC time.Time
}

// Tracker holds daily input/output token counters keyed by tenantKey.
type Tracker struct {
	mu      sync.Mutex
	tenants map[string]*dailyCounters
	maxIn   int
	maxOut  int
	now     func() time.Time
}

// New constructs a Tracker. maxIn/maxOut may be 0 (= unlimited; Reserve always
// returns ok=true; the tracker still records totals for metrics).
// now is injectable for tests (production: time.Now).
func New(maxInputTokens, maxOutputTokens int, now func() time.Time) *Tracker {
	if now == nil {
		now = time.Now
	}
	return &Tracker{
		tenants: make(map[string]*dailyCounters),
		maxIn:   maxInputTokens,
		maxOut:  maxOutputTokens,
		now:     now,
	}
}

// Reserve checks the budget BEFORE a /turn LLM call.
// estIn / estOut are conservative caller estimates.
// On reject, retryAfter = (next-00:00-UTC - now) rounded UP to seconds, floor 1.
// Reserve does NOT mutate counters (Commit does, after SSEDone).
func (t *Tracker) Reserve(tenantKey string, estIn, estOut int) (ok bool, retryAfter time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	c := t.tenantLocked(tenantKey, now)

	// Unlimited mode (maxIn=0, maxOut=0): always allow.
	if t.maxIn == 0 && t.maxOut == 0 {
		return true, 0
	}

	// Either cap exceeded by the projected total → reject.
	if t.maxIn > 0 && c.in+estIn > t.maxIn {
		return false, secsToNextUTCMidnight(now)
	}
	if t.maxOut > 0 && c.out+estOut > t.maxOut {
		return false, secsToNextUTCMidnight(now)
	}
	return true, 0
}

// Commit records the actual usage AFTER SSEDone fires. Never rejects.
func (t *Tracker) Commit(tenantKey string, actualIn, actualOut int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	c := t.tenantLocked(tenantKey, now)
	c.in += actualIn
	c.out += actualOut
}

// Snapshot returns the current counters for tenantKey for metrics export.
// Returns zero values when the tenant has no recorded counters yet (the
// returned dayStartUTC is the zero Time in that case).
func (t *Tracker) Snapshot(tenantKey string) (in, out int, dayStartUTC time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	c, ok := t.tenants[tenantKey]
	if !ok {
		return 0, 0, time.Time{}
	}
	// Snapshot honors the UTC day boundary too — a stale entry from yesterday
	// returns (0, 0, today).
	now := t.now()
	today := now.UTC().Truncate(24 * time.Hour)
	if c.dayStartUTC.Before(today) {
		return 0, 0, today
	}
	return c.in, c.out, c.dayStartUTC
}

// tenantLocked returns the counter for tenantKey, resetting it if the active
// day has rolled past midnight UTC. Caller MUST hold t.mu.
func (t *Tracker) tenantLocked(tenantKey string, now time.Time) *dailyCounters {
	today := now.UTC().Truncate(24 * time.Hour)
	c, ok := t.tenants[tenantKey]
	if !ok {
		c = &dailyCounters{dayStartUTC: today}
		t.tenants[tenantKey] = c
		return c
	}
	if c.dayStartUTC.Before(today) {
		// New UTC day — reset.
		c.in = 0
		c.out = 0
		c.dayStartUTC = today
	}
	return c
}

// secsToNextUTCMidnight returns the duration until the next 00:00 UTC,
// rounded UP to whole seconds and floored at 1s (HTTP Retry-After numeric form).
func secsToNextUTCMidnight(now time.Time) time.Duration {
	nextDay := now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
	d := nextDay.Sub(now)
	if d <= 0 {
		return time.Second
	}
	secs := math.Ceil(d.Seconds())
	if secs < 1 {
		secs = 1
	}
	return time.Duration(secs) * time.Second
}
