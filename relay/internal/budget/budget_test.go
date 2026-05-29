package budget_test

import (
	"testing"
	"time"

	"intake/internal/budget"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
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

func TestTracker_AtCapBoundary_AllowsExactlyAtCapRejectsOneOver(t *testing.T) {
	// The over-cap check uses strict `>`, not `>=`. This means:
	// - Committing exactly to cap is fine; the next Reserve with estIn=0 passes.
	// - The next Reserve with any positive estIn rejects.
	// Test fixes the "soft daily budget" semantic against future refactors.
	c := newClock()
	tr := budget.New(100, 100, c.Now)
	tr.Commit("", 100, 0) // exactly at input cap

	// Reserve with estIn=0 must still pass — c.in+0 == maxIn, NOT > maxIn.
	ok, _ := tr.Reserve("", 0, 0)
	if !ok {
		t.Error("Reserve(0,0) at-cap rejected; strict '>' check should allow exactly-at-cap")
	}
	// Reserve with estIn=1 must reject — c.in+1 > maxIn.
	ok, _ = tr.Reserve("", 1, 0)
	if ok {
		t.Error("Reserve(1,0) one-over-cap allowed; strict '>' check should reject")
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
