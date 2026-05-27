package auth

import (
	"sync"

	"github.com/google/uuid"
)

// Store is a thread-safe in-memory session store.
//
// Phase 1: sessions never expire. TTL and per-session token/turn caps
// are a Phase 5 concern — add a map[string]sessionMeta with timestamps
// and a background eviction goroutine at that point.
type Store struct {
	mu       sync.RWMutex
	sessions map[string]struct{}
}

// NewStore returns a ready-to-use Store.
func NewStore() *Store {
	return &Store{sessions: make(map[string]struct{})}
}

// Issue mints a new session ID (UUID v4), records it, and returns it.
func (s *Store) Issue() string {
	id := uuid.New().String()
	s.mu.Lock()
	s.sessions[id] = struct{}{}
	s.mu.Unlock()
	return id
}

// Validate reports whether id was issued by this store.
func (s *Store) Validate(id string) bool {
	s.mu.RLock()
	_, ok := s.sessions[id]
	s.mu.RUnlock()
	return ok
}
