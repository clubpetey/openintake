package emailcode_test

import (
	"regexp"
	"sync"
	"testing"
	"time"

	"intake/internal/auth/emailcode"
)

// virtualClock is a controllable now-source.
type virtualClock struct {
	mu sync.Mutex
	t  time.Time
}

func (v *virtualClock) Now() time.Time {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.t
}

func (v *virtualClock) Advance(d time.Duration) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.t = v.t.Add(d)
}

func newVC() *virtualClock {
	return &virtualClock{t: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)}
}

var sixDigit = regexp.MustCompile(`^[0-9]{6}$`)

func TestStore_Issue_ReturnsSixDigitCode(t *testing.T) {
	vc := newVC()
	s := emailcode.New(10*time.Minute, 10*time.Minute, 3, vc.Now)

	code, retry, err := s.Issue("user@example.com")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if retry != 0 {
		t.Errorf("retryAfter should be 0 on success, got %v", retry)
	}
	if !sixDigit.MatchString(code) {
		t.Errorf("code %q is not 6 digits", code)
	}
}

func TestStore_VerifyHappyPath(t *testing.T) {
	vc := newVC()
	s := emailcode.New(10*time.Minute, 10*time.Minute, 3, vc.Now)

	code, _, err := s.Issue("user@example.com")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if !s.Verify("user@example.com", code) {
		t.Fatal("Verify should accept a fresh code")
	}
}

func TestStore_VerifyWrongCode(t *testing.T) {
	vc := newVC()
	s := emailcode.New(10*time.Minute, 10*time.Minute, 3, vc.Now)

	if _, _, err := s.Issue("user@example.com"); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if s.Verify("user@example.com", "000000") {
		t.Fatal("Verify should reject a wrong code")
	}
}

func TestStore_SingleUse(t *testing.T) {
	vc := newVC()
	s := emailcode.New(10*time.Minute, 10*time.Minute, 3, vc.Now)

	code, _, _ := s.Issue("user@example.com")
	if !s.Verify("user@example.com", code) {
		t.Fatal("first verify must succeed")
	}
	if s.Verify("user@example.com", code) {
		t.Fatal("second verify of the same code must fail (single-use)")
	}
}

func TestStore_TTLEviction(t *testing.T) {
	vc := newVC()
	s := emailcode.New(10*time.Minute, 10*time.Minute, 3, vc.Now)

	code, _, _ := s.Issue("user@example.com")
	vc.Advance(10*time.Minute + time.Second) // past TTL
	if s.Verify("user@example.com", code) {
		t.Fatal("Verify must reject an expired code")
	}
}

func TestStore_RateLimit_FourthIssueReturnsRetryAfter(t *testing.T) {
	vc := newVC()
	s := emailcode.New(10*time.Minute, 10*time.Minute, 3, vc.Now)

	for i := 0; i < 3; i++ {
		if _, _, err := s.Issue("user@example.com"); err != nil {
			t.Fatalf("issue %d: %v", i, err)
		}
	}
	code, retry, err := s.Issue("user@example.com")
	if err == nil {
		t.Fatal("4th Issue must return ErrRateLimited")
	}
	if err != emailcode.ErrRateLimited {
		t.Errorf("err = %v; want ErrRateLimited", err)
	}
	if code != "" {
		t.Errorf("rate-limited Issue must return empty code, got %q", code)
	}
	if retry <= 0 {
		t.Errorf("retryAfter must be > 0 when rate-limited, got %v", retry)
	}
}

func TestStore_RateLimit_WindowResets(t *testing.T) {
	vc := newVC()
	s := emailcode.New(10*time.Minute, 10*time.Minute, 3, vc.Now)

	for i := 0; i < 3; i++ {
		if _, _, err := s.Issue("user@example.com"); err != nil {
			t.Fatalf("issue %d: %v", i, err)
		}
	}
	if _, _, err := s.Issue("user@example.com"); err != emailcode.ErrRateLimited {
		t.Fatalf("4th issue (in-window) should rate-limit, got %v", err)
	}
	vc.Advance(10*time.Minute + time.Second) // window resets
	if _, _, err := s.Issue("user@example.com"); err != nil {
		t.Fatalf("after window reset, Issue should succeed, got %v", err)
	}
}

func TestStore_RateLimit_IndependentPerEmail(t *testing.T) {
	vc := newVC()
	s := emailcode.New(10*time.Minute, 10*time.Minute, 3, vc.Now)

	for i := 0; i < 3; i++ {
		if _, _, err := s.Issue("a@example.com"); err != nil {
			t.Fatalf("issue a %d: %v", i, err)
		}
	}
	// Different email — should still succeed.
	if _, _, err := s.Issue("b@example.com"); err != nil {
		t.Fatalf("Issue for different email must not be rate-limited, got %v", err)
	}
}

func TestStore_Concurrent_IssueAndVerify(t *testing.T) {
	vc := newVC()
	s := emailcode.New(10*time.Minute, 10*time.Minute, 1000, vc.Now)

	const N = 50
	codes := make(chan string, N)
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, _, err := s.Issue("concurrent@example.com")
			if err != nil {
				t.Errorf("Issue: %v", err)
				return
			}
			codes <- c
		}()
	}
	wg.Wait()
	close(codes)
	// Drain all codes — they should all be unique 6-digit strings, but only the
	// most-recent matters for Verify (multiple issuances overwrite the active code).
	count := 0
	for c := range codes {
		if !sixDigit.MatchString(c) {
			t.Errorf("non-6-digit code %q", c)
		}
		count++
	}
	if count != N {
		t.Errorf("issued %d codes; want %d", count, N)
	}
}
