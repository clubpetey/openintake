package adapter

// Truncate returns s limited to max runes, appending "…" when it was shortened.
// Rune-aware so it never splits a multibyte UTF-8 character (these strings are
// downstream error/response bodies included in adapter error messages).
func Truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
