package adapter

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncate_ShortReturnedUnchanged(t *testing.T) {
	s := "hello"
	got := Truncate(s, 10)
	if got != s {
		t.Errorf("expected %q unchanged, got %q", s, got)
	}
}

func TestTruncate_ExactLengthReturnedUnchanged(t *testing.T) {
	s := "hello"
	got := Truncate(s, 5)
	if got != s {
		t.Errorf("expected %q unchanged at exact max, got %q", s, got)
	}
}

func TestTruncate_LongASCIITruncated(t *testing.T) {
	s := strings.Repeat("a", 300)
	got := Truncate(s, 200)
	want := strings.Repeat("a", 200) + "…"
	if got != want {
		t.Errorf("expected %d 'a's + ellipsis, got len=%d", 200, len([]rune(got)))
	}
}

func TestTruncate_MultibyteNoMidRuneSplit(t *testing.T) {
	// "é" is a 2-byte UTF-8 rune; 10 of them = 20 bytes, truncate to 5 runes.
	s := strings.Repeat("é", 10)
	got := Truncate(s, 5)

	wantRunes := strings.Repeat("é", 5) + "…"
	if got != wantRunes {
		t.Errorf("expected %q, got %q", wantRunes, got)
	}
	if !utf8.ValidString(got) {
		t.Errorf("result is not valid UTF-8: %q", got)
	}
}

func TestTruncate_EmptyString(t *testing.T) {
	got := Truncate("", 10)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}
