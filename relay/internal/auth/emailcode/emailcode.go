// Package emailcode is a thread-safe, in-memory store of pending email
// verification codes. Each email may hold at most one active code at a time
// (most recent overwrites prior); rate-limit caps the number of Issue calls
// per email within a rolling window.
//
// Storage is in-memory only — restart invalidates pending codes (acceptable
// for v0; design spec §11 non-goal). Eviction is eager on Issue/Verify (no
// background goroutine).
package emailcode

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// ErrRateLimited is returned by Issue when the per-email rate-limit cap is hit.
var ErrRateLimited = errors.New("emailcode: rate-limited")

// entry holds a single issued code and its lifecycle metadata.
type entry struct {
	code     string
	issuedAt time.Time
	used     bool
}

// Store is the in-memory code store.
type Store struct {
	mu sync.Mutex

	codeTTL      time.Duration
	rateWindow   time.Duration
	perWindowCap int
	now          func() time.Time

	// active maps email → its single most-recent (unconsumed, unexpired) code.
	active map[string]*entry

	// history maps email → ordered timestamps of Issue calls within rateWindow.
	// Entries older than rateWindow are evicted on each Issue/Verify call.
	history map[string][]time.Time
}

// New returns a Store with the given code TTL, rate-limit window, and per-window cap.
// `now` is injectable for tests (production: time.Now).
func New(codeTTL, rateWindow time.Duration, perWindowCap int, now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{
		codeTTL:      codeTTL,
		rateWindow:   rateWindow,
		perWindowCap: perWindowCap,
		now:          now,
		active:       make(map[string]*entry),
		history:      make(map[string][]time.Time),
	}
}

// Issue generates a fresh 6-digit code for email. If ≥perWindowCap codes have
// been issued for email within the last rateWindow, returns ("", retryAfter,
// ErrRateLimited) where retryAfter is the time until the oldest still-in-window
// issuance ages out. On success, stores the code (overwriting any prior active
// entry for email) and returns the code.
func (s *Store) Issue(email string) (string, time.Duration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()

	// Evict history entries outside the rate window.
	hist := s.history[email]
	cutoff := now.Add(-s.rateWindow)
	pruned := hist[:0]
	for _, t := range hist {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	s.history[email] = pruned

	if len(pruned) >= s.perWindowCap {
		// retryAfter: the oldest in-window issuance ages out at hist[0] + rateWindow.
		retry := pruned[0].Add(s.rateWindow).Sub(now)
		if retry < time.Second {
			retry = time.Second // caller writes Retry-After header in seconds; never advertise 0
		}
		return "", retry, ErrRateLimited
	}

	// Evict any active-entry that has expired (for any email — eager cleanup
	// keeps memory bounded). Inline since callers are infrequent.
	for k, e := range s.active {
		if now.Sub(e.issuedAt) > s.codeTTL {
			delete(s.active, k)
		}
	}

	code, err := generateSixDigitCode()
	if err != nil {
		return "", 0, fmt.Errorf("emailcode: generate: %w", err)
	}

	s.active[email] = &entry{code: code, issuedAt: now, used: false}
	s.history[email] = append(s.history[email], now)
	return code, 0, nil
}

// Verify reports whether code matches an unexpired, unused issuance for email.
// On match, marks it used (single-use); subsequent verifies of the same code fail.
func (s *Store) Verify(email, code string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	e, ok := s.active[email]
	if !ok {
		return false
	}
	if e.used {
		return false
	}
	if now.Sub(e.issuedAt) > s.codeTTL {
		delete(s.active, email)
		return false
	}
	if e.code != code {
		return false
	}
	e.used = true
	return true
}

// generateSixDigitCode returns a uniformly-distributed 6-digit decimal string
// using crypto/rand. Range: [000000, 999999] inclusive.
func generateSixDigitCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
