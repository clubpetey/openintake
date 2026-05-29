package perip_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"intake/internal/ratelimit/perip"
)

// fakeClock returns time.Time values from a controllable counter.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
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
		t.Errorf("retry-after = %v; want >=1s (floor)", retry)
	}

	// Advance 2 seconds — at 1 req/sec, >=2 tokens should be refilled.
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
		t.Errorf("post-GC map len = %d; want <=5 (most idle buckets evicted)", got)
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

func TestLimiter_RetryAfterFlooredAtOneSecond(t *testing.T) {
	c := newClock()
	// 10 req/s would refill in 100ms; assertion is that retry-after is still
	// >=1s (RFC 9110 numeric-form Retry-After floor).
	l := perip.New(10.0, 1, 15*time.Minute, c.Now)

	l.Allow("1.1.1.1") // consume the 1
	_, retry := l.Allow("1.1.1.1")
	if retry < time.Second {
		t.Errorf("retry-after = %v; want >=1s (floor; never below 1s for HTTP Retry-After)", retry)
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
	// Produce a unique-per-i IPv4-shaped string. Two octets vary so i up to
	// 65535 stays unique (the provided letters-mod-16 form collided every 16).
	return fmt.Sprintf("10.0.%d.%d", (i>>8)&0xff, i&0xff)
}
