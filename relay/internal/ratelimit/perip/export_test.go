package perip

// MapLen exposes the internal map length for tests. Not part of the public API.
func MapLen(l *Limiter) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}
