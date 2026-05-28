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

	// Note: cap check uses >= because meta.turns is the count of COMPLETED
	// turns (RecordTurn already fired after the previous SSEDone). budget.Tracker
	// uses > because Reserve adds a not-yet-charged estimate. Both reject one
	// turn over the policy cap; the operator differs because the counter timing
	// differs.
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
//
// Negative inputTokens are clamped to 0 — a per-session token accumulator
// must not be decrementable by a buggy or compromised caller, which would
// silently increase the available headroom under the maxInputTokens cap
// (a rate-limit-bypass primitive). Mirrors budget.Tracker.Commit's clamp.
func (s *Store) RecordTurn(id string, inputTokens int) {
	if inputTokens < 0 {
		inputTokens = 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, ok := s.sessionMeta[id]
	if !ok {
		return
	}
	meta.turns++
	meta.cumInputTokens += inputTokens
}
