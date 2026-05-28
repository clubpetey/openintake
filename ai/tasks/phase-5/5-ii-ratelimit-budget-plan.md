# 5-ii Per-IP Limiter + Per-Session Counters + Daily Budget Tracker — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the three 5-i stubs with their real implementations. (1) `perip.Limiter` — `golang.org/x/time/rate`-backed per-key token bucket with eager-GC by idle TTL. (2) `auth.Store` extension — `NewStoreWithCaps` + `CheckSession` + `RecordTurn` for the 20-turn / 8000-token / 1-hour caps. (3) `budget.Tracker` — daily-keyed input/output counters with Reserve/Commit semantics, UTC midnight reset, per-tenant isolation. Wire all three into `turnHandler` (per-session check before LLM call → Reserve → stream → Commit + RecordTurn on SSEDone) and into `main.go` (real `perip.New`, `budget.New`, `auth.NewStoreWithCaps`). After this sub-plan: a /turn over the per-IP burst returns 429+Retry-After:1; the 21st /turn in a session returns 429 session_turns_exhausted; a /turn that would push the daily budget over returns 503 daily_budget_exhausted+Retry-After:<secs-to-midnight>.

**Architecture:** Three credit-free in-memory primitives, all using the L014 injectable-clock pattern. Each is a small package with one type, one constructor, and 2-3 methods. `perip.Limiter` wraps `rate.Limiter` per-key with a `sync.Mutex`-guarded map + eager GC (no background goroutine). `budget.Tracker` holds `map[tenantKey]*dailyCounters`; every Reserve/Commit checks the UTC day boundary and resets on rollover. `auth.Store` gains a private `sessionMeta` map and three new methods; `NewStore()` stays as a wrapper around `NewStoreWithCaps(0, 0, 0, time.Now)` so Phase 1+4 tests are unaffected. `turnHandler` orders the checks: per-session (Store) first → budget Reserve second → provider.Chat third → on SSEDone: budget.Commit + Store.RecordTurn. An aborted stream means no Commit, no RecordTurn — the user is not charged for failed turns.

**Tech Stack:** Go 1.23.2 (relay). `golang.org/x/time/rate` (promoted to direct in 5-i). No other new dependencies.

---

## Design References

- README §8.3 — `perip.Limiter` shape (frozen here)
- README §8.4 — `budget.Tracker` shape (frozen here)
- README §8.6 — `auth.Store` extension (frozen here)
- README §8.9 — endpoint contracts (the 429s + 503 land here)
- Design spec §3.1 — `perip.Limiter` behavior
- Design spec §3.2 — `budget.Tracker` Reserve/Commit semantics + UTC reset
- Design spec §3.4 — `auth.Store` sessionMeta + CheckSession + RecordTurn
- Design spec §6.2 — `/turn` data flow (check → Reserve → stream → Commit + RecordTurn)
- Reference: existing `relay/internal/auth/emailcode/emailcode.go` — the canonical pattern for injectable clock + eager-eviction (L014)
- Reference: existing `relay/internal/server/turn.go:72-159` (Phase 1+4 `turnHandler` — Phase 5 inserts gates at lines ~75 + uses SSEDone token counts at ~145)

---

## Files Touched

| File | Action | Responsibility |
|---|---|---|
| `relay/internal/ratelimit/perip/perip.go` | Modify | Replace 5-i placeholder with full Limiter implementation |
| `relay/internal/ratelimit/perip/perip_test.go` | Create | Injected-clock unit tests: burst→reject→refill, GC eviction, concurrent safety, zero-rate edge case |
| `relay/internal/budget/budget.go` | Modify | Replace 5-i placeholder with full Tracker (Reserve/Commit/Snapshot) |
| `relay/internal/budget/budget_test.go` | Create | Injected-clock unit tests: under-cap→Reserve, at-cap→reject with right secs-to-midnight, Commit increments, UTC rollover resets, tenants isolated, unlimited mode |
| `relay/internal/auth/store.go` | Modify | Add `NewStoreWithCaps`, sessionMeta map, `CheckSession`, `RecordTurn`; preserve `NewStore` as wrapper |
| `relay/internal/auth/store_test.go` | Modify | Add tests for cap enforcement + TTL expiry + RecordTurn behavior |
| `relay/internal/server/turn.go` | Modify | Insert per-session check + budget Reserve before provider.Chat; Commit + RecordTurn on SSEDone |
| `relay/internal/server/turn_test.go` | Modify | Tests for 429 session-exhausted / 503 budget-exhausted / 401 session-expired |
| `relay/cmd/relay/main.go` | Modify | Construct real Limiter + Tracker + NewStoreWithCaps; populate Deps |

---

## Tasks

### Task 1: Implement `perip.Limiter` with `rate.Limiter`-per-key + eager GC

**Files:** Modify `relay/internal/ratelimit/perip/perip.go`, Create `relay/internal/ratelimit/perip/perip_test.go`

- [ ] **Step 1: Write the failing tests**

Create `relay/internal/ratelimit/perip/perip_test.go`:

```go
package perip_test

import (
	"sync"
	"testing"
	"time"

	"intake/internal/ratelimit/perip"
)

// fakeClock returns time.Time values from a controllable counter.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }
func (c *fakeClock) advance(d time.Duration) { c.now = c.now.Add(d) }

func newClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func TestLimiter_BurstAllowedThenRejectedThenRecharges(t *testing.T) {
	c := newClock()
	l := perip.New(1.0, 5, 15*time.Minute, c.Now)

	// Burst of 5 must all pass at t0.
	for i := 0; i < 5; i++ {
		ok, retry := l.Allow("203.0.113.1")
		if !ok {
			t.Fatalf("Allow #%d in burst rejected (retry=%v); want ok", i+1, retry)
		}
	}
	// The 6th must reject.
	ok, retry := l.Allow("203.0.113.1")
	if ok {
		t.Fatalf("Allow #6 (post-burst) returned ok; want reject")
	}
	if retry < time.Second {
		t.Errorf("retry-after = %v; want ≥1s (floor)", retry)
	}

	// Advance 2 seconds — at 1 req/sec, ≥2 tokens should be refilled.
	c.advance(2 * time.Second)
	for i := 0; i < 2; i++ {
		ok, _ := l.Allow("203.0.113.1")
		if !ok {
			t.Fatalf("Allow post-refill #%d rejected; want ok (advanced 2s at 1 req/s)", i+1)
		}
	}
}

func TestLimiter_DifferentIPsHaveIndependentBuckets(t *testing.T) {
	c := newClock()
	l := perip.New(1.0, 2, 15*time.Minute, c.Now)

	// Exhaust IP A's burst.
	l.Allow("1.1.1.1")
	l.Allow("1.1.1.1")
	ok, _ := l.Allow("1.1.1.1")
	if ok {
		t.Fatal("IP A 3rd request allowed; want reject (burst=2)")
	}
	// IP B must still be fresh.
	ok, _ = l.Allow("2.2.2.2")
	if !ok {
		t.Fatal("IP B 1st request rejected; want ok (independent bucket)")
	}
}

func TestLimiter_EagerGC_EvictsIdleBuckets(t *testing.T) {
	c := newClock()
	l := perip.New(1.0, 5, 1*time.Minute, c.Now)

	// Touch many IPs.
	for i := 0; i < 100; i++ {
		l.Allow(makeIP(i))
	}
	if got := perip.MapLen(l); got != 100 {
		t.Errorf("post-fill map len = %d; want 100", got)
	}

	// Advance past idle TTL; any further Allow should sweep expired buckets.
	c.advance(2 * time.Minute)
	l.Allow("1.1.1.1") // trigger GC pass
	if got := perip.MapLen(l); got > 5 {
		t.Errorf("post-GC map len = %d; want ≤5 (most idle buckets evicted)", got)
	}
}

func TestLimiter_EmptyIP_SharedBucket(t *testing.T) {
	c := newClock()
	l := perip.New(1.0, 1, 15*time.Minute, c.Now)

	// Empty IP shares one bucket — burst of 1, then reject.
	ok, _ := l.Allow("")
	if !ok {
		t.Fatal("empty IP 1st request rejected")
	}
	ok, _ = l.Allow("")
	if ok {
		t.Fatal("empty IP 2nd request allowed; want reject (single shared bucket)")
	}
}

func TestLimiter_RetryAfterRespectsRateForSubSecond(t *testing.T) {
	c := newClock()
	l := perip.New(10.0, 1, 15*time.Minute, c.Now) // 10 req/s = 100ms refill

	l.Allow("1.1.1.1") // consume the 1
	_, retry := l.Allow("1.1.1.1")
	if retry < time.Second {
		t.Errorf("retry-after = %v; want ≥1s (floor; never below 1s for HTTP Retry-After)", retry)
	}
}

func TestLimiter_ConcurrentAllowSafe(t *testing.T) {
	c := newClock()
	l := perip.New(1000.0, 100, 15*time.Minute, c.Now)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				l.Allow(makeIP(id))
			}
		}(i)
	}
	wg.Wait()
	// No panic, no data race (run with `go test -race`).
}

func makeIP(i int) string {
	const letters = "0123456789abcdef"
	return string(letters[i%16]) + ".1.1." + string(letters[(i*7)%16])
}
```

The test file uses `perip.MapLen(l)` — a test-only helper. Add it via a same-package test file (avoids exposing internals to production callers).

Create `relay/internal/ratelimit/perip/export_test.go`:

```go
package perip

// MapLen exposes the internal map length for tests. Not part of the public API.
func MapLen(l *Limiter) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./internal/ratelimit/perip/... -v && cd ..`
Expected: FAIL — current 5-i placeholder doesn't enforce burst, doesn't GC, doesn't have a `buckets` field.

- [ ] **Step 3: Replace `perip.go` with the full implementation**

Overwrite `relay/internal/ratelimit/perip/perip.go`:

```go
// Package perip is the per-IP token-bucket rate limiter for the relay's
// /v1/intake/* endpoints. Backed by golang.org/x/time/rate.Limiter per key,
// with eager GC of buckets whose last-seen exceeds idleTTL.
//
// L014: injectable clock for deterministic tests; eager-eviction matches
// the auth/emailcode pattern (no background goroutine).
package perip

import (
	"math"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// entry holds one client IP's rate.Limiter and the last time it was touched.
type entry struct {
	bucket   *rate.Limiter
	lastSeen time.Time
}

// Limiter holds a per-client-IP token bucket. Buckets are eagerly GC'd when
// a key's last-seen exceeds the configured idle TTL.
type Limiter struct {
	mu        sync.Mutex
	buckets   map[string]*entry
	rate      rate.Limit // events per second
	burst     int
	idleTTL   time.Duration
	now       func() time.Time
	retryWait time.Duration // pre-computed: 1/rate, floor 1s
}

// New constructs a Limiter. reqsPerSecond is the steady-state rate; burst is
// the bucket capacity. idleTTL is how long an unused key is retained before GC.
// now is injectable for tests (production: time.Now).
//
// reqsPerSecond ≤ 0 is treated as "no rate" — every Allow returns ok=true
// (degraded safe; main.go's startup gate should have rejected this config,
// but the constructor remains permissive).
func New(reqsPerSecond float64, burst int, idleTTL time.Duration, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	r := rate.Limit(reqsPerSecond)
	// retryWait: how long until the bucket has another token at the configured rate.
	// At rate=0, retryWait is irrelevant (Allow always returns ok=true below).
	retry := time.Second
	if reqsPerSecond > 0 {
		retry = time.Duration(math.Ceil(1.0/reqsPerSecond*float64(time.Second))) * time.Nanosecond
		// Floor at 1s for HTTP Retry-After numeric form (RFC 9110).
		if retry < time.Second {
			retry = time.Second
		}
	}
	return &Limiter{
		buckets:   make(map[string]*entry),
		rate:      r,
		burst:     burst,
		idleTTL:   idleTTL,
		now:       now,
		retryWait: retry,
	}
}

// Allow reports whether ip may proceed now. On reject, retryAfter is the bucket
// refill interval (1/reqsPerSecond) rounded UP to seconds, floor 1.
// An empty ip shares a single bucket (degraded safe; caller's clientIPMiddleware
// stashed "" because RemoteAddr was unparseable).
func (l *Limiter) Allow(ip string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.gcLocked(now)

	if l.rate <= 0 {
		// "No rate" → always allow; still track lastSeen for consistency.
		e, exists := l.buckets[ip]
		if !exists {
			e = &entry{bucket: rate.NewLimiter(rate.Inf, l.burst), lastSeen: now}
			l.buckets[ip] = e
		}
		e.lastSeen = now
		return true, 0
	}

	e, exists := l.buckets[ip]
	if !exists {
		e = &entry{bucket: rate.NewLimiter(l.rate, l.burst), lastSeen: now}
		l.buckets[ip] = e
	}
	e.lastSeen = now

	// rate.Limiter is internally clocked from time.Now; for deterministic tests
	// we use AllowN(now, 1) which consumes from the bucket at the supplied time.
	if e.bucket.AllowN(now, 1) {
		return true, 0
	}
	return false, l.retryWait
}

// gcLocked evicts buckets whose lastSeen is older than idleTTL.
// Caller MUST hold l.mu.
func (l *Limiter) gcLocked(now time.Time) {
	if l.idleTTL <= 0 {
		return
	}
	cutoff := now.Add(-l.idleTTL)
	for k, e := range l.buckets {
		if e.lastSeen.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
}
```

Delete the file `relay/internal/ratelimit/perip/perip.go` first (the 5-i placeholder) and re-create with the above content — or overwrite in place; both yield the same diff.

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test -race ./internal/ratelimit/perip/... -v && cd ..`
Expected: all 6 tests pass under `-race`.

- [ ] **Step 5: Commit**

```bash
git add relay/internal/ratelimit/perip/perip.go relay/internal/ratelimit/perip/perip_test.go relay/internal/ratelimit/perip/export_test.go
git commit -m "feat(5-ii): perip.Limiter — rate.Limiter-per-key + eager GC + injected clock"
```

---

### Task 2: Implement `budget.Tracker` (Reserve / Commit / Snapshot)

**Files:** Modify `relay/internal/budget/budget.go`, Create `relay/internal/budget/budget_test.go`

- [ ] **Step 1: Write the failing tests**

Create `relay/internal/budget/budget_test.go`:

```go
package budget_test

import (
	"testing"
	"time"

	"intake/internal/budget"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time         { return c.now }
func (c *fakeClock) advance(d time.Duration) { c.now = c.now.Add(d) }

// Start at noon UTC on 2026-01-01.
func newClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
}

func TestTracker_ReserveUnderCap_AllowsAndDoesNotMutate(t *testing.T) {
	c := newClock()
	tr := budget.New(1000, 1000, c.Now)

	ok, retry := tr.Reserve("", 100, 100)
	if !ok {
		t.Fatalf("Reserve(100,100) under 1000/1000 cap rejected; retry=%v", retry)
	}
	// Reserve must NOT mutate; a second identical Reserve must still pass.
	ok, _ = tr.Reserve("", 100, 100)
	if !ok {
		t.Error("2nd Reserve(100,100) rejected; Reserve should not mutate counters")
	}
	in, out, _ := tr.Snapshot("")
	if in != 0 || out != 0 {
		t.Errorf("counters after Reserve = (%d,%d); want (0,0)", in, out)
	}
}

func TestTracker_CommitIncrementsCounters(t *testing.T) {
	c := newClock()
	tr := budget.New(1000, 1000, c.Now)

	tr.Commit("", 50, 25)
	tr.Commit("", 50, 25)
	in, out, _ := tr.Snapshot("")
	if in != 100 || out != 50 {
		t.Errorf("counters = (%d,%d); want (100,50)", in, out)
	}
}

func TestTracker_ReserveOverCap_Rejects(t *testing.T) {
	c := newClock()
	tr := budget.New(100, 100, c.Now)
	tr.Commit("", 90, 0)

	ok, retry := tr.Reserve("", 20, 0) // would push to 110 > 100
	if ok {
		t.Error("Reserve over input cap allowed; want reject")
	}
	if retry <= 0 {
		t.Errorf("retry-after = %v; want >0 (some seconds-to-midnight)", retry)
	}
	// retry-after should be ~12 hours (noon to next midnight UTC).
	expectedMin := 11 * time.Hour
	expectedMax := 13 * time.Hour
	if retry < expectedMin || retry > expectedMax {
		t.Errorf("retry-after = %v; want between %v and %v (≈12h to midnight UTC)", retry, expectedMin, expectedMax)
	}
}

func TestTracker_OutputCapAlsoEnforced(t *testing.T) {
	c := newClock()
	tr := budget.New(1_000_000, 100, c.Now)
	tr.Commit("", 0, 90)

	ok, _ := tr.Reserve("", 0, 20)
	if ok {
		t.Error("Reserve over output cap allowed; want reject")
	}
}

func TestTracker_UTCMidnightRolloverResets(t *testing.T) {
	c := newClock() // noon Jan 1
	tr := budget.New(100, 100, c.Now)
	tr.Commit("", 90, 90)

	// Advance to 12:01am Jan 2 UTC — past midnight.
	c.advance(12*time.Hour + 1*time.Minute)

	ok, _ := tr.Reserve("", 50, 50)
	if !ok {
		t.Error("Reserve after UTC midnight rollover rejected; want ok (counters should reset)")
	}
	in, out, _ := tr.Snapshot("")
	if in != 0 || out != 0 {
		t.Errorf("counters after rollover = (%d,%d); want (0,0)", in, out)
	}
}

func TestTracker_TenantsAreIsolated(t *testing.T) {
	c := newClock()
	tr := budget.New(100, 100, c.Now)
	tr.Commit("tenant-A", 90, 90)

	// Tenant A is near cap; tenant B should still pass.
	ok, _ := tr.Reserve("tenant-B", 50, 50)
	if !ok {
		t.Error("Tenant B Reserve rejected by Tenant A's usage; tenants must be isolated")
	}
	// Empty tenant key (no X-Intake-Tenant header) is yet another isolated bucket.
	ok, _ = tr.Reserve("", 50, 50)
	if !ok {
		t.Error("Empty tenant Reserve rejected by Tenant A's usage")
	}
}

func TestTracker_UnlimitedMode_AlwaysAllows(t *testing.T) {
	c := newClock()
	tr := budget.New(0, 0, c.Now) // 0/0 → unlimited

	ok, _ := tr.Reserve("", 1_000_000_000, 1_000_000_000)
	if !ok {
		t.Error("Reserve with maxIn=0,maxOut=0 rejected; 0 means unlimited")
	}
	// Counters still record for metrics.
	tr.Commit("", 100, 100)
	in, out, _ := tr.Snapshot("")
	if in != 100 || out != 100 {
		t.Errorf("counters = (%d,%d); want (100,100) — unlimited mode still records", in, out)
	}
}

func TestTracker_RetryAfterFloorOneSecond(t *testing.T) {
	// Pin the clock to one second before UTC midnight. Reserve over-cap should
	// return retryAfter ≥1s even though wall time is sub-second.
	c := &fakeClock{now: time.Date(2026, 1, 1, 23, 59, 59, 500_000_000, time.UTC)}
	tr := budget.New(100, 100, c.Now)
	tr.Commit("", 95, 95)

	_, retry := tr.Reserve("", 10, 10)
	if retry < time.Second {
		t.Errorf("retry-after = %v; want ≥1s (floor for HTTP Retry-After)", retry)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./internal/budget/... -v && cd ..`
Expected: FAIL — current placeholder Reserve always returns ok=true; Commit is a no-op; Snapshot returns zeros.

- [ ] **Step 3: Replace `budget.go` with the full implementation**

Overwrite `relay/internal/budget/budget.go`:

```go
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
```

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test -race ./internal/budget/... -v && cd ..`
Expected: all 8 tests pass.

- [ ] **Step 5: Commit**

```bash
git add relay/internal/budget/budget.go relay/internal/budget/budget_test.go
git commit -m "feat(5-ii): budget.Tracker — Reserve/Commit/Snapshot + UTC reset + tenant isolation"
```

---

### Task 3: Extend `auth.Store` with `NewStoreWithCaps` + `CheckSession` + `RecordTurn`

**Files:** Modify `relay/internal/auth/store.go`, `relay/internal/auth/store_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `relay/internal/auth/store_test.go`:

```go
import (
	// existing imports + add:
	"time"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time         { return c.now }
func (c *fakeClock) advance(d time.Duration) { c.now = c.now.Add(d) }

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
}

func TestStoreWithCaps_FreshSessionCanTakeTurns(t *testing.T) {
	c := newFakeClock()
	s := auth.NewStoreWithCaps(20, 8000, time.Hour, c.Now)
	id := s.Issue()

	ok, _, code := s.CheckSession(id)
	if !ok {
		t.Fatalf("CheckSession on fresh session = (false, %s); want ok", code)
	}
}

func TestStoreWithCaps_TurnsCapAt20(t *testing.T) {
	c := newFakeClock()
	s := auth.NewStoreWithCaps(20, 0, time.Hour, c.Now)
	id := s.Issue()

	for i := 0; i < 20; i++ {
		ok, _, _ := s.CheckSession(id)
		if !ok {
			t.Fatalf("Check #%d rejected; want ok (under cap)", i+1)
		}
		s.RecordTurn(id, 100)
	}
	ok, retry, code := s.CheckSession(id)
	if ok {
		t.Fatal("21st check allowed; want reject")
	}
	if code != "session_turns_exhausted" {
		t.Errorf("code = %q; want session_turns_exhausted", code)
	}
	if retry <= 0 {
		t.Errorf("retry-after = %v; want >0", retry)
	}
}

func TestStoreWithCaps_TokensCapAt8000(t *testing.T) {
	c := newFakeClock()
	s := auth.NewStoreWithCaps(0, 8000, time.Hour, c.Now)
	id := s.Issue()

	// 4 turns × 2000 input tokens each = exactly at cap. 5th must reject.
	for i := 0; i < 4; i++ {
		ok, _, _ := s.CheckSession(id)
		if !ok {
			t.Fatalf("Check #%d rejected; want ok", i+1)
		}
		s.RecordTurn(id, 2000)
	}
	ok, _, code := s.CheckSession(id)
	if ok {
		t.Fatal("5th check after 8000 cumulative tokens allowed; want reject")
	}
	if code != "session_tokens_exhausted" {
		t.Errorf("code = %q; want session_tokens_exhausted", code)
	}
}

func TestStoreWithCaps_TTLExpiry(t *testing.T) {
	c := newFakeClock()
	s := auth.NewStoreWithCaps(20, 8000, time.Hour, c.Now)
	id := s.Issue()

	c.advance(61 * time.Minute) // past 1h TTL
	ok, _, code := s.CheckSession(id)
	if ok {
		t.Fatal("CheckSession after TTL allowed; want reject")
	}
	if code != "session_expired" {
		t.Errorf("code = %q; want session_expired", code)
	}
	// Expired sessions evicted on read — Validate must also return false.
	if s.Validate(id) {
		t.Error("Validate after expiry = true; want false")
	}
}

func TestStoreWithCaps_ZeroCapsMeansNoLimit(t *testing.T) {
	c := newFakeClock()
	s := auth.NewStoreWithCaps(0, 0, 0, c.Now) // no caps, no TTL
	id := s.Issue()

	for i := 0; i < 1000; i++ {
		ok, _, _ := s.CheckSession(id)
		if !ok {
			t.Fatalf("Check #%d rejected with no caps", i+1)
		}
		s.RecordTurn(id, 100)
	}
}

func TestStoreWithCaps_RecordTurnOnUnknownIDIsNoOp(t *testing.T) {
	c := newFakeClock()
	s := auth.NewStoreWithCaps(20, 8000, time.Hour, c.Now)
	s.RecordTurn("not-a-real-id", 100) // must not panic
}

func TestStoreWithCaps_CheckSessionOnUnknownIDIsExpired(t *testing.T) {
	c := newFakeClock()
	s := auth.NewStoreWithCaps(20, 8000, time.Hour, c.Now)
	ok, _, code := s.CheckSession("not-a-real-id")
	if ok {
		t.Fatal("CheckSession on unknown id allowed; want reject")
	}
	if code != "session_expired" {
		t.Errorf("code = %q; want session_expired (treat unknown as expired)", code)
	}
}

func TestNewStore_PreservesPhase1Behavior(t *testing.T) {
	s := auth.NewStore() // Phase 1 constructor — no caps, no TTL, time.Now
	id := s.Issue()
	if !s.Validate(id) {
		t.Fatal("Phase 1 Validate failed")
	}
	for i := 0; i < 1000; i++ {
		ok, _, _ := s.CheckSession(id)
		if !ok {
			t.Fatalf("CheckSession #%d rejected; Phase 1 NewStore must have no caps", i+1)
		}
		s.RecordTurn(id, 100)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./internal/auth/ -run Store -v && cd ..`
Expected: FAIL — `NewStoreWithCaps`, `CheckSession`, `RecordTurn` undefined.

- [ ] **Step 3: Modify `store.go` with the extension**

Overwrite `relay/internal/auth/store.go`:

```go
package auth

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// sessionMeta holds per-session counters and TTL state (Phase 5).
type sessionMeta struct {
	createdAt      time.Time
	turns          int
	cumInputTokens int
}

// Store is a thread-safe in-memory session store.
//
// Phase 1: anonymous-session lookup only (Issue/Validate).
// Phase 5: per-session turn/token caps + 1-hour TTL via sessionMeta.
// Phase 1+4 callers using NewStore() see zero behavior change because
// NewStore wraps NewStoreWithCaps(0, 0, 0, time.Now) — zero caps mean
// no limit; zero TTL means no expiry.
type Store struct {
	mu             sync.RWMutex
	sessions       map[string]struct{}
	sessionMeta    map[string]*sessionMeta
	maxTurns       int
	maxInputTokens int
	sessionTTL     time.Duration
	now            func() time.Time
}

// NewStore returns a ready-to-use Store with Phase 1 semantics (no caps,
// no TTL). Preserved as a wrapper around NewStoreWithCaps so Phase 1+4
// callers see zero behavior change.
func NewStore() *Store {
	return NewStoreWithCaps(0, 0, 0, time.Now)
}

// NewStoreWithCaps is the Phase 5 constructor.
// maxTurns / maxInputTokens / sessionTTL all 0 → no cap / no TTL
// (Phase 1+4 backward-compat for tests).
// now is injectable for tests (production: time.Now).
func NewStoreWithCaps(maxTurns, maxInputTokens int, sessionTTL time.Duration, now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{
		sessions:       make(map[string]struct{}),
		sessionMeta:    make(map[string]*sessionMeta),
		maxTurns:       maxTurns,
		maxInputTokens: maxInputTokens,
		sessionTTL:     sessionTTL,
		now:            now,
	}
}

// Issue mints a new session ID (UUID v4), records it, and returns it.
// Phase 5: also creates the per-session counter entry.
func (s *Store) Issue() string {
	id := uuid.New().String()
	s.mu.Lock()
	s.sessions[id] = struct{}{}
	s.sessionMeta[id] = &sessionMeta{createdAt: s.now()}
	s.mu.Unlock()
	return id
}

// Validate reports whether id was issued by this store and (Phase 5) has not
// expired. Expired sessions are evicted on read.
func (s *Store) Validate(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[id]; !ok {
		return false
	}
	if s.sessionTTL > 0 {
		if meta, ok := s.sessionMeta[id]; ok {
			if s.now().Sub(meta.createdAt) >= s.sessionTTL {
				delete(s.sessions, id)
				delete(s.sessionMeta, id)
				return false
			}
		}
	}
	return true
}

// CheckSession reports whether the session may take another turn.
// On reject:
//   - code == "session_expired" when id is unknown OR createdAt+TTL has passed
//     (retryAfter is 0 — caller must re-init).
//   - code == "session_turns_exhausted" when turns >= maxTurns
//     (retryAfter is the remaining TTL).
//   - code == "session_tokens_exhausted" when cumInputTokens >= maxInputTokens
//     (retryAfter is the remaining TTL).
//
// Caps of 0 mean "no limit" for that dimension.
func (s *Store) CheckSession(id string) (ok bool, retryAfter time.Duration, code string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, exists := s.sessionMeta[id]
	if !exists {
		return false, 0, "session_expired"
	}

	now := s.now()

	// TTL check first.
	if s.sessionTTL > 0 {
		age := now.Sub(meta.createdAt)
		if age >= s.sessionTTL {
			delete(s.sessions, id)
			delete(s.sessionMeta, id)
			return false, 0, "session_expired"
		}
		// remaining TTL for the rate-limit retry-after.
		retryAfter = s.sessionTTL - age
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
	}

	if s.maxTurns > 0 && meta.turns >= s.maxTurns {
		return false, retryAfter, "session_turns_exhausted"
	}
	if s.maxInputTokens > 0 && meta.cumInputTokens >= s.maxInputTokens {
		return false, retryAfter, "session_tokens_exhausted"
	}
	return true, 0, ""
}

// RecordTurn increments turns by 1 and adds inputTokens to cumInputTokens
// for session id. No-op if id is unknown.
func (s *Store) RecordTurn(id string, inputTokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, ok := s.sessionMeta[id]
	if !ok {
		return
	}
	meta.turns++
	meta.cumInputTokens += inputTokens
}
```

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test ./internal/auth/ -v && cd ..`
Expected: all existing tests + the new Phase 5 tests pass.

- [ ] **Step 5: Commit**

```bash
git add relay/internal/auth/store.go relay/internal/auth/store_test.go
git commit -m "feat(5-ii): auth.Store gains NewStoreWithCaps + CheckSession + RecordTurn"
```

---

### Task 4: Insert per-session check + budget Reserve/Commit into `turnHandler`

**Files:** Modify `relay/internal/server/turn.go`, `relay/internal/server/turn_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `relay/internal/server/turn_test.go`:

```go
// fakeProvider is a minimal llm.Provider that emits a single SSEDone with
// caller-controlled input/output token counts. Used by Phase 5 /turn tests
// so the budget Commit + Store RecordTurn paths can be exercised without
// hitting a real LLM.
type fakeProvider struct {
	inputTokens  int
	outputTokens int
}

func (p *fakeProvider) Name() string { return "fake" }
func (p *fakeProvider) Chat(ctx context.Context, msgs []llm.Message, opts llm.ChatOptions) (<-chan llm.Chunk, error) {
	ch := make(chan llm.Chunk, 1)
	ch <- llm.Chunk{Done: true, InputTokens: p.inputTokens, OutputTokens: p.outputTokens}
	close(ch)
	return ch, nil
}

func TestTurnHandler_SessionTurnsExhausted_Returns429(t *testing.T) {
	store := auth.NewStoreWithCaps(2, 0, time.Hour, time.Now)
	id := store.Issue()
	// Pre-fill turns to cap.
	store.RecordTurn(id, 0)
	store.RecordTurn(id, 0)

	deps := Deps{
		Auth:     auth.NewMiddleware(store, nil, nil),
		Provider: &fakeProvider{inputTokens: 10, outputTokens: 10},
		Model:    "test",
		MaxTokens: 100,
	}
	h := turnHandler(deps)

	req := httptest.NewRequest("POST", "/v1/intake/turn", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	// Attach SessionContext via WithSession (bypass the middleware in this unit test).
	ctx := auth.WithSession(req.Context(), &auth.SessionContext{SessionID: id, AuthMode: "anonymous"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d; want 429", rec.Code)
	}
	var body ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "session_turns_exhausted" {
		t.Errorf("code = %q; want session_turns_exhausted", body.Error.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("Retry-After header missing on 429")
	}
}

func TestTurnHandler_SessionExpired_Returns401(t *testing.T) {
	store := auth.NewStoreWithCaps(20, 8000, time.Hour, time.Now)
	// SessionID that was never issued.
	id := "00000000-0000-0000-0000-000000000000"

	deps := Deps{
		Auth:     auth.NewMiddleware(store, nil, nil),
		Provider: &fakeProvider{},
		Model:    "test",
		MaxTokens: 100,
	}
	h := turnHandler(deps)
	req := httptest.NewRequest("POST", "/v1/intake/turn", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	ctx := auth.WithSession(req.Context(), &auth.SessionContext{SessionID: id, AuthMode: "anonymous"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", rec.Code)
	}
	var body ErrorEnvelope
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Error.Code != "session_expired" {
		t.Errorf("code = %q; want session_expired", body.Error.Code)
	}
}

func TestTurnHandler_BudgetExhausted_Returns503(t *testing.T) {
	store := auth.NewStoreWithCaps(0, 0, 0, time.Now) // no session caps
	id := store.Issue()

	tracker := budget.New(100, 100, time.Now)
	tracker.Commit("", 95, 95) // near cap

	deps := Deps{
		Auth:      auth.NewMiddleware(store, nil, nil),
		Provider:  &fakeProvider{inputTokens: 10, outputTokens: 10},
		Model:     "test",
		MaxTokens: 100,
		Budget:    tracker,
	}
	h := turnHandler(deps)
	req := httptest.NewRequest("POST", "/v1/intake/turn", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	ctx := auth.WithSession(req.Context(), &auth.SessionContext{SessionID: id, AuthMode: "anonymous"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; want 503", rec.Code)
	}
	var body ErrorEnvelope
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Error.Code != "daily_budget_exhausted" {
		t.Errorf("code = %q; want daily_budget_exhausted", body.Error.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("Retry-After header missing on 503")
	}
}

func TestTurnHandler_CommitOnSSEDone_RecordsTokens(t *testing.T) {
	store := auth.NewStoreWithCaps(20, 8000, time.Hour, time.Now)
	id := store.Issue()
	tracker := budget.New(10000, 10000, time.Now)

	deps := Deps{
		Auth:      auth.NewMiddleware(store, nil, nil),
		Provider:  &fakeProvider{inputTokens: 50, outputTokens: 25},
		Model:     "test",
		MaxTokens: 100,
		Budget:    tracker,
	}
	h := turnHandler(deps)
	req := httptest.NewRequest("POST", "/v1/intake/turn", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	ctx := auth.WithSession(req.Context(), &auth.SessionContext{SessionID: id, AuthMode: "anonymous"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (SSE)", rec.Code)
	}
	// budget.Commit must have fired.
	in, out, _ := tracker.Snapshot("")
	if in != 50 || out != 25 {
		t.Errorf("budget after Commit = (%d,%d); want (50,25)", in, out)
	}
	// auth.Store.RecordTurn must have fired.
	ok, _, _ := store.CheckSession(id)
	if !ok {
		t.Fatal("post-Commit CheckSession rejected; should still pass (1 of 20 turns used)")
	}
	// One more validation: a second turn should also pass and increment.
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req.WithContext(ctx))
	in2, out2, _ := tracker.Snapshot("")
	if in2 != 100 || out2 != 50 {
		t.Errorf("budget after 2nd Commit = (%d,%d); want (100,50)", in2, out2)
	}
}

func TestTurnHandler_TenantKeyFromHeader(t *testing.T) {
	store := auth.NewStoreWithCaps(0, 0, 0, time.Now)
	id := store.Issue()
	tracker := budget.New(10000, 10000, time.Now)

	deps := Deps{
		Auth:      auth.NewMiddleware(store, nil, nil),
		Provider:  &fakeProvider{inputTokens: 10, outputTokens: 5},
		Model:     "test",
		MaxTokens: 100,
		Budget:    tracker,
	}
	h := turnHandler(deps)
	req := httptest.NewRequest("POST", "/v1/intake/turn", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("X-Intake-Tenant", "acme")
	ctx := auth.WithSession(req.Context(), &auth.SessionContext{SessionID: id, AuthMode: "anonymous"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	// Counters must land under tenant "acme", not the empty key.
	in, _, _ := tracker.Snapshot("acme")
	if in != 10 {
		t.Errorf("tenant acme input = %d; want 10", in)
	}
	inEmpty, _, _ := tracker.Snapshot("")
	if inEmpty != 0 {
		t.Errorf("empty tenant input = %d; want 0 (request had X-Intake-Tenant)", inEmpty)
	}
}
```

Add to the existing `turn_test.go` imports: `"context"`, `"intake/internal/budget"`, `"intake/internal/llm"`.

- [ ] **Step 2: Run to verify they fail**

Run: `cd relay && go test ./internal/server/ -run TestTurnHandler -v && cd ..`
Expected: FAIL — turnHandler currently does not check session caps or budget; it does not call Commit/RecordTurn after SSEDone.

- [ ] **Step 3: Replace `turnHandler` body with the gated version**

In `relay/internal/server/turn.go`, replace the existing `turnHandler` function:

```go
// turnHandler handles POST /v1/intake/turn (behind the auth middleware).
//
// Phase 5 gating, applied in order:
//   1. Resolve SessionContext (already in ctx from auth middleware).
//   2. deps.Auth.Store().CheckSession — 429 session_turns/_tokens_exhausted
//      or 401 session_expired on reject.
//   3. deps.Budget.Reserve — 503 daily_budget_exhausted on reject.
//   4. provider.Chat (Phase 1+4 unchanged).
//   5. On SSEDone: deps.Budget.Commit + deps.Auth.Store().RecordTurn.
//
// Reserve uses conservative estimates (4-chars/token input; deps.MaxTokens out)
// so the gate trips BEFORE the LLM call. An aborted stream means no Commit
// and no RecordTurn — failed turns don't count against the user.
func turnHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req TurnRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid request body: "+err.Error())
			return
		}

		sess, _ := auth.FromContext(r.Context())
		if sess == nil {
			writeError(w, http.StatusInternalServerError, "internal", "session context missing")
			return
		}

		// Phase 5 gate 1: per-session caps.
		if deps.Auth != nil {
			ok, retryAfter, code := deps.Auth.Store().CheckSession(sess.SessionID)
			if !ok {
				if code == "session_expired" {
					writeError(w, http.StatusUnauthorized, "session_expired", "session expired; call POST /v1/intake/init again")
					return
				}
				secs := int(retryAfter.Seconds())
				if secs < 1 {
					secs = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				var msg string
				if code == "session_turns_exhausted" {
					msg = "session turn limit reached"
				} else {
					msg = "session input-token limit reached"
				}
				writeError(w, http.StatusTooManyRequests, code, msg)
				return
			}
		}

		// Phase 5 gate 2: daily LLM budget.
		tenantKey := r.Header.Get("X-Intake-Tenant")
		estIn := approximateInputTokens(req.Messages)
		estOut := deps.MaxTokens
		if deps.Budget != nil {
			ok, retryAfter := deps.Budget.Reserve(tenantKey, estIn, estOut)
			if !ok {
				secs := int(retryAfter.Seconds())
				if secs < 1 {
					secs = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				writeError(w, http.StatusServiceUnavailable, "daily_budget_exhausted", "relay daily LLM budget reached")
				return
			}
		}

		// Build the message list for the provider.
		msgs := make([]llm.Message, 0, len(req.Messages)+1)
		if deps.SystemPrompt != "" {
			msgs = append(msgs, llm.Message{Role: "system", Content: deps.SystemPrompt})
		}
		for _, m := range req.Messages {
			msgs = append(msgs, llm.Message{Role: m.Role, Content: m.Content})
		}

		if deps.Provider == nil {
			writeError(w, http.StatusInternalServerError, "internal", "provider not configured")
			return
		}

		opts := llm.ChatOptions{
			Model:     deps.Model,
			MaxTokens: deps.MaxTokens,
			Stream:    true,
		}

		ch, err := deps.Provider.Chat(r.Context(), msgs, opts)
		if err != nil {
			slog.ErrorContext(r.Context(), "provider chat failed", "error", err)
			writeError(w, http.StatusBadGateway, "provider_error", "upstream provider unavailable")
			return
		}

		// SSE headers.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		flusher, canFlush := w.(http.Flusher)
		ctx := r.Context()

		for {
			select {
			case <-ctx.Done():
				go func() {
					for range ch {
					}
				}()
				return
			case chunk, ok := <-ch:
				if !ok {
					return
				}
				if chunk.Err != nil {
					writeSSEFrame(w, SSEError{Error: chunk.Err.Error()})
					if canFlush {
						flusher.Flush()
					}
					return
				}
				if chunk.Delta != "" {
					writeSSEFrame(w, SSEDelta{Delta: chunk.Delta})
					if canFlush {
						flusher.Flush()
					}
				}
				if chunk.Done {
					// Phase 5: Commit budget + RecordTurn on successful SSEDone.
					if deps.Budget != nil {
						deps.Budget.Commit(tenantKey, chunk.InputTokens, chunk.OutputTokens)
					}
					if deps.Auth != nil {
						deps.Auth.Store().RecordTurn(sess.SessionID, chunk.InputTokens)
					}
					writeSSEFrame(w, SSEDone{
						Done:         true,
						InputTokens:  chunk.InputTokens,
						OutputTokens: chunk.OutputTokens,
					})
					if canFlush {
						flusher.Flush()
					}
					return
				}
			}
		}
	}
}

// approximateInputTokens returns a conservative estimate of the input-token
// count of the messages, used by budget.Reserve as the pre-flight estimate.
// Uses a simple 4-chars-per-token heuristic; the actual count comes back in
// SSEDone and replaces this estimate at Commit time.
func approximateInputTokens(msgs []TurnMessage) int {
	const charsPerToken = 4
	total := 0
	for _, m := range msgs {
		total += (len(m.Content) + charsPerToken - 1) / charsPerToken
	}
	return total
}
```

Add the imports: `"strconv"`, `"intake/internal/auth"`. (`"intake/internal/llm"` is already imported.)

- [ ] **Step 4: Run the tests — must pass**

Run: `cd relay && go test ./internal/server/ -v && cd ..`
Expected: all existing server tests + the 5 new TestTurnHandler tests pass.

Run: `cd relay && go test ./... && cd ..`
Expected: ALL relay tests pass.

- [ ] **Step 5: Commit**

```bash
git add relay/internal/server/turn.go relay/internal/server/turn_test.go
git commit -m "feat(5-ii): turnHandler gates — CheckSession → budget.Reserve → Chat → Commit + RecordTurn"
```

---

### Task 5: Wire real instances in `main.go`

**Files:** Modify `relay/cmd/relay/main.go`

- [ ] **Step 1: Replace `auth.NewStore()` with `auth.NewStoreWithCaps(...)`**

In `main.go`, find this line (around line 90):

```go
	store := auth.NewStore()
```

Replace with:

```go
	// Phase 5 (5-ii): Store gains per-session caps + TTL from cfg.RateLimit.PerSession.
	sessionTTL, err := time.ParseDuration(cfg.RateLimit.PerSession.SessionTTL)
	if err != nil {
		logger.Error("relay: invalid ratelimit.per_session.session_ttl", "value", cfg.RateLimit.PerSession.SessionTTL, "err", err)
		os.Exit(1)
	}
	store := auth.NewStoreWithCaps(
		cfg.RateLimit.PerSession.MaxTurns,
		cfg.RateLimit.PerSession.MaxInputTokens,
		sessionTTL,
		time.Now,
	)
```

- [ ] **Step 2: Construct the real `perip.Limiter` and `budget.Tracker`**

Find the existing Deps construction (around line 232). BEFORE the `deps := server.Deps{...}` block, ADD:

```go
	// Phase 5 (5-ii): per-IP rate limiter + daily-budget tracker.
	idleTTL, err := time.ParseDuration(cfg.RateLimit.PerIP.IdleTTL)
	if err != nil {
		logger.Error("relay: invalid ratelimit.per_ip.idle_ttl", "value", cfg.RateLimit.PerIP.IdleTTL, "err", err)
		os.Exit(1)
	}
	perIPLimiter := perip.New(
		cfg.RateLimit.PerIP.RequestsPerSecond,
		cfg.RateLimit.PerIP.Burst,
		idleTTL,
		time.Now,
	)
	budgetTracker := budget.New(
		cfg.RateLimit.DailyLLMBudget.MaxInputTokens,
		cfg.RateLimit.DailyLLMBudget.MaxOutputTokens,
		time.Now,
	)
	logger.Info("relay: rate limits configured",
		"per_ip_rps", cfg.RateLimit.PerIP.RequestsPerSecond,
		"per_ip_burst", cfg.RateLimit.PerIP.Burst,
		"per_session_max_turns", cfg.RateLimit.PerSession.MaxTurns,
		"per_session_max_input_tokens", cfg.RateLimit.PerSession.MaxInputTokens,
		"daily_budget_max_input_tokens", cfg.RateLimit.DailyLLMBudget.MaxInputTokens,
		"daily_budget_max_output_tokens", cfg.RateLimit.DailyLLMBudget.MaxOutputTokens,
	)
```

In the `deps := server.Deps{...}` block, REPLACE the two nil assignments with the real instances. Find these lines:

```go
		CaptchaCfg:      cfg.Captcha,
		CaptchaVerifier: nil,
		Budget:          nil,
		PerIP:           nil,
		TrustedProxies:  trustedProxies,
```

Replace with:

```go
		CaptchaCfg:      cfg.Captcha,
		CaptchaVerifier: nil,             // 5-iii lands the real verifier
		Budget:          budgetTracker,    // 5-ii
		PerIP:           perIPLimiter,     // 5-ii
		TrustedProxies:  trustedProxies,
```

Add the imports at the top of `main.go` if not already present:

```go
import (
	// ... existing ...
	"intake/internal/budget"
	"intake/internal/ratelimit/perip"
)
```

- [ ] **Step 3: Build + vet + tests**

Run: `cd relay && go build ./... && go vet ./... && cd ..`
Expected: passes.

Run: `cd relay && go test ./... && cd ..`
Expected: ALL relay tests pass.

Run: `bash scripts/verify-contract.sh`
Expected: exits 0.

- [ ] **Step 4: Commit**

```bash
git add relay/cmd/relay/main.go
git commit -m "feat(5-ii): wire real perip.Limiter + budget.Tracker + NewStoreWithCaps from cfg"
```

---

## Smoke (mandatory)

**Self-runnable; no LLM credit; no maintainer pause.** Uses a fake provider (or local Ollama if available) for the /turn paths so the LLM call doesn't actually spend credit.

1. **Per-IP burst smoke** (`relay/cmd/relay/smoke/perip-burst.sh`):

   Start the relay with `ratelimit.per_ip = {requests_per_second:1, burst:5}` + the Q9 escape hatch.
   ```bash
   for i in $(seq 1 10); do
     curl -s -o /dev/null -w '%{http_code}\n' -X POST http://127.0.0.1:18080/v1/intake/init -d '{}'
   done
   ```
   Expected: first 5 lines `200`, next 5 lines `429`. Verify `Retry-After: 1` on the 429 via `-D-`.

   Control: same curl loop against `/v1/health` → all 10 lines `200` (probes are not rate-limited).

2. **Per-session cap smoke** (`relay/cmd/relay/smoke/per-session-cap.sh`):

   Start the relay with `ratelimit.per_session.max_turns:3` (small cap for speed) + the Q9 escape hatch.
   ```bash
   SESSION=$(curl -s -X POST http://127.0.0.1:18080/v1/intake/init -d '{}' | jq -r .session_id)
   for i in $(seq 1 4); do
     curl -s -o /dev/null -w '%{http_code} ' -X POST http://127.0.0.1:18080/v1/intake/turn \
       -H "X-Intake-Session: $SESSION" \
       -d '{"messages":[{"role":"user","content":"hi"}]}'
   done; echo
   ```
   Expected: `200 200 200 429` — the 4th returns 429 with `session_turns_exhausted` and a `Retry-After` close to the session TTL.

   This step requires either a local Ollama OR the build-time `OLLAMA_BASE_URL` env redirected to a tiny stub HTTP server that returns an SSE `done` immediately. The 5-iv driver script includes both options.

3. **Daily budget smoke** (`relay/cmd/relay/smoke/budget.sh`):

   Start the relay with `ratelimit.daily_llm_budget.max_input_tokens:100` + Ollama-or-stub provider.
   First /turn: succeeds and burns ~50 input tokens.
   Second /turn: same request — should still pass (50+50=100 → at cap, but Reserve uses `>`-cap not `>=`-cap so we need to confirm the boundary).
   Third /turn: fails 503 with `daily_budget_exhausted` and `Retry-After: <secs-to-midnight>`.

4. **Two-tenant isolation smoke** (`relay/cmd/relay/smoke/tenant-isolation.sh`):

   With `max_input_tokens:100`: exhaust tenant `acme` (X-Intake-Tenant: acme) → fail. Then a single turn with `X-Intake-Tenant: beta` succeeds. Proves tenants don't share buckets.

## Done criteria

- [ ] All 5 tasks complete and committed.
- [ ] `cd relay && go build ./... && go vet ./...` clean.
- [ ] `cd relay && go test -race ./...` green (all Phase 1+4 + Phase 5-i + Phase 5-ii tests pass under `-race`).
- [ ] `bash scripts/verify-contract.sh` green.
- [ ] `bash scripts/check-pins.sh` green.
- [ ] All four smoke steps pass.
- [ ] `Phase 1 NewStore()` path still works (`TestNewStore_PreservesPhase1Behavior` passes — backward-compat preserved).
- [ ] No new external Go module added (`go mod tidy` is a no-op).
