package auth_test

import (
	"testing"
	"time"

	"intake/internal/auth"
)

func TestStore_IssueAndValidate(t *testing.T) {
	s := auth.NewStore()
	id := s.Issue()
	if id == "" {
		t.Fatal("Issue() returned empty string")
	}
	if !s.Validate(id) {
		t.Errorf("Validate(%q) = false; want true", id)
	}
}

func TestStore_UnknownIDFails(t *testing.T) {
	s := auth.NewStore()
	if s.Validate("not-a-real-session") {
		t.Error("Validate(unknown) = true; want false")
	}
}

func TestStore_IssueIsUnique(t *testing.T) {
	s := auth.NewStore()
	a := s.Issue()
	b := s.Issue()
	if a == b {
		t.Errorf("Issue() returned identical IDs: %q", a)
	}
}

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
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
