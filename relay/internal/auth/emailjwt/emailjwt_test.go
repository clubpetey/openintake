package emailjwt_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"intake/internal/auth/emailjwt"

	"github.com/golang-jwt/jwt/v5"
)

// thirtyTwoByteSecret returns a deterministic 32-byte secret for tests.
var thirtyTwoByteSecret = []byte("0123456789abcdef0123456789abcdef")

func TestMint_RejectsShortSecret(t *testing.T) {
	short := []byte("too-short")
	_, _, err := emailjwt.Mint(short, "user@example.com", 15*time.Minute)
	if err == nil {
		t.Fatal("Mint must reject a secret shorter than 32 bytes")
	}
	if !strings.Contains(err.Error(), "32") {
		t.Errorf("error should mention the 32-byte minimum, got %v", err)
	}
}

func TestMintVerify_RoundTrip(t *testing.T) {
	token, expiresAt, err := emailjwt.Mint(thirtyTwoByteSecret, "user@example.com", 15*time.Minute)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if token == "" {
		t.Fatal("Mint returned empty token")
	}
	if !expiresAt.After(time.Now()) {
		t.Errorf("expiresAt %v must be in the future", expiresAt)
	}

	email, err := emailjwt.Verify(thirtyTwoByteSecret, token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if email != "user@example.com" {
		t.Errorf("email = %q; want user@example.com", email)
	}
}

func TestVerify_RejectsTamperedToken(t *testing.T) {
	token, _, err := emailjwt.Mint(thirtyTwoByteSecret, "user@example.com", 15*time.Minute)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	// Flip a character in the signature segment (last dot-separated piece).
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("unexpected token shape: %d parts", len(parts))
	}
	last := []byte(parts[2])
	if last[0] == 'A' {
		last[0] = 'B'
	} else {
		last[0] = 'A'
	}
	tampered := parts[0] + "." + parts[1] + "." + string(last)
	if _, err := emailjwt.Verify(thirtyTwoByteSecret, tampered); err == nil {
		t.Fatal("Verify must reject a tampered token")
	}
}

func TestVerify_RejectsWrongSecret(t *testing.T) {
	token, _, err := emailjwt.Mint(thirtyTwoByteSecret, "user@example.com", 15*time.Minute)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	other := []byte("ffffffffffffffffffffffffffffffff") // also 32 bytes, but different
	if _, err := emailjwt.Verify(other, token); err == nil {
		t.Fatal("Verify must reject a token signed with a different secret")
	}
}

func TestVerify_RejectsExpiredToken(t *testing.T) {
	// Mint with a tiny negative TTL via direct claims (the public Mint takes a
	// positive ttl); craft an already-expired token using jwt directly.
	claims := jwt.MapClaims{
		"sub": "user@example.com",
		"iat": time.Now().Add(-time.Hour).Unix(),
		"exp": time.Now().Add(-time.Minute).Unix(), // expired
		"iss": emailjwt.Issuer,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(thirtyTwoByteSecret)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	if _, err := emailjwt.Verify(thirtyTwoByteSecret, signed); err == nil {
		t.Fatal("Verify must reject an expired token")
	}
}

func TestVerify_RejectsWrongIssuer(t *testing.T) {
	// Mint a token with iss="other-system" — Verify must reject so an SSO token
	// minted with the same secret can never sneak through the email branch.
	claims := jwt.MapClaims{
		"sub": "user@example.com",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(15 * time.Minute).Unix(),
		"iss": "other-system",
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(thirtyTwoByteSecret)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	if _, err := emailjwt.Verify(thirtyTwoByteSecret, signed); err == nil {
		t.Fatal("Verify must reject a token whose iss is not 'intake-email'")
	}
}

func TestVerify_RejectsEmptySub(t *testing.T) {
	claims := jwt.MapClaims{
		"sub": "",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(15 * time.Minute).Unix(),
		"iss": emailjwt.Issuer,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := tok.SignedString(thirtyTwoByteSecret)
	if _, err := emailjwt.Verify(thirtyTwoByteSecret, signed); err == nil {
		t.Fatal("Verify must reject a token with empty sub")
	}
}

func TestVerify_RejectsNonHS256AlgHeader(t *testing.T) {
	// Hand-craft a token whose header claims alg=RS256 but whose signature is
	// HMAC-signed with the secret. Verify must reject it because the
	// *jwt.SigningMethodHMAC type-assertion in the key-func fails.
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims := map[string]any{
		"sub": "attacker@example.com",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(15 * time.Minute).Unix(),
		"iss": emailjwt.Issuer,
	}
	claimsJSON, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	mac := hmac.New(sha256.New, thirtyTwoByteSecret)
	mac.Write([]byte(header + "." + payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	crafted := header + "." + payload + "." + sig

	if _, err := emailjwt.Verify(thirtyTwoByteSecret, crafted); err == nil {
		t.Fatal("Verify must reject a token whose header claims alg=RS256")
	}
}

func TestVerifier_AdapterSatisfiesInterface(t *testing.T) {
	v := &emailjwt.Verifier{Secret: thirtyTwoByteSecret}
	token, _, err := emailjwt.Mint(thirtyTwoByteSecret, "user@example.com", time.Minute)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	email, err := v.Verify(token)
	if err != nil {
		t.Fatalf("Verifier.Verify: %v", err)
	}
	if email != "user@example.com" {
		t.Errorf("email = %q; want user@example.com", email)
	}
}
