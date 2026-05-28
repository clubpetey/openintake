package sso_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"intake/internal/auth/sso"
	"intake/internal/config"
)

// rsaKid is a deterministic kid used in all RS256 tests.
const rsaKid = "test-kid-001"

// rsaFixture bundles an in-test RSA keypair, a JWKS server that serves the
// matching public key, and a helper to mint signed tokens.
type rsaFixture struct {
	priv    *rsa.PrivateKey
	jwksURL string
	server  *httptest.Server
}

func newRSAFixture(t *testing.T) *rsaFixture {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	jwks := buildJWKS(t, priv, rsaKid)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwks)
	}))
	return &rsaFixture{priv: priv, jwksURL: srv.URL + "/.well-known/jwks.json", server: srv}
}

func (f *rsaFixture) close() { f.server.Close() }

// buildJWKS returns a JWKS JSON containing the RSA public key as the sole entry.
func buildJWKS(t *testing.T, priv *rsa.PrivateKey, kid string) []byte {
	t.Helper()
	n := base64.RawURLEncoding.EncodeToString(priv.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.E)).Bytes())
	doc := map[string]any{
		"keys": []map[string]any{
			{
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"kid": kid,
				"n":   n,
				"e":   e,
			},
		},
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}
	return b
}

// mintRS256 signs claims with priv using the given kid in the header.
func mintRS256(t *testing.T, priv *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(priv)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	return s
}

// validClaims returns a fresh, valid claim set targeting the standard test
// issuer/audience.
func validClaims() jwt.MapClaims {
	now := time.Now().Unix()
	return jwt.MapClaims{
		"iss":   "https://issuer.test/",
		"aud":   "https://api.test",
		"exp":   now + 3600,
		"iat":   now,
		"nbf":   now - 5,
		"sub":   "user-abc-123",
		"email": "user@example.com",
		"name":  "Test User",
	}
}

// cfgFor returns an SSOConfig pointed at the fixture's JWKS URL.
func cfgFor(jwksURL string) config.SSOConfig {
	return config.SSOConfig{
		Issuer:   "https://issuer.test/",
		Audience: "https://api.test",
		JWKSURL:  jwksURL,
		Claims:   config.SSOClaimNames{UserID: "sub", Email: "email", DisplayName: "name"},
	}
}

func TestRS256_HappyPath(t *testing.T) {
	f := newRSAFixture(t)
	defer f.close()

	v, err := sso.NewRS256Verifier(cfgFor(f.jwksURL), silentLogger())
	if err != nil {
		t.Fatalf("NewRS256Verifier: %v", err)
	}
	tok := mintRS256(t, f.priv, rsaKid, validClaims())

	claims, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.UserID != "user-abc-123" {
		t.Errorf("UserID = %q; want user-abc-123", claims.UserID)
	}
	if claims.Email == nil || *claims.Email != "user@example.com" {
		t.Errorf("Email = %v; want user@example.com", claims.Email)
	}
	if claims.DisplayName == nil || *claims.DisplayName != "Test User" {
		t.Errorf("DisplayName = %v; want Test User", claims.DisplayName)
	}
}

func TestRS256_TamperedToken_Rejected(t *testing.T) {
	f := newRSAFixture(t)
	defer f.close()
	v, err := sso.NewRS256Verifier(cfgFor(f.jwksURL), silentLogger())
	if err != nil {
		t.Fatalf("NewRS256Verifier: %v", err)
	}

	tok := mintRS256(t, f.priv, rsaKid, validClaims())
	// Flip a byte in the payload section (between the two dots).
	tampered := []byte(tok)
	for i := range tampered {
		if tampered[i] == '.' {
			// Mutate the next byte if available.
			if i+1 < len(tampered) {
				if tampered[i+1] == 'A' {
					tampered[i+1] = 'B'
				} else {
					tampered[i+1] = 'A'
				}
				break
			}
		}
	}
	_, err = v.Verify(context.Background(), string(tampered))
	if err == nil {
		t.Fatal("expected error for tampered token")
	}
}

func TestRS256_WrongKid_Rejected(t *testing.T) {
	f := newRSAFixture(t)
	defer f.close()
	v, err := sso.NewRS256Verifier(cfgFor(f.jwksURL), silentLogger())
	if err != nil {
		t.Fatalf("NewRS256Verifier: %v", err)
	}

	// Mint a token signed by a DIFFERENT keypair with the same kid — JWKS won't
	// have a key whose public bytes match.
	otherPriv, _ := rsa.GenerateKey(rand.Reader, 2048)
	tok := mintRS256(t, otherPriv, rsaKid, validClaims())
	_, err = v.Verify(context.Background(), tok)
	if err == nil {
		t.Fatal("expected error for token signed with non-JWKS key")
	}
}

func TestRS256_Expired_Rejected(t *testing.T) {
	f := newRSAFixture(t)
	defer f.close()
	v, err := sso.NewRS256Verifier(cfgFor(f.jwksURL), silentLogger())
	if err != nil {
		t.Fatalf("NewRS256Verifier: %v", err)
	}

	claims := validClaims()
	// 1 hour ago — well past the 30s clock skew.
	claims["exp"] = time.Now().Add(-1 * time.Hour).Unix()
	tok := mintRS256(t, f.priv, rsaKid, claims)
	_, err = v.Verify(context.Background(), tok)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestRS256_WrongIssuer_Rejected(t *testing.T) {
	f := newRSAFixture(t)
	defer f.close()
	v, err := sso.NewRS256Verifier(cfgFor(f.jwksURL), silentLogger())
	if err != nil {
		t.Fatalf("NewRS256Verifier: %v", err)
	}

	claims := validClaims()
	claims["iss"] = "https://attacker.test/"
	tok := mintRS256(t, f.priv, rsaKid, claims)
	_, err = v.Verify(context.Background(), tok)
	if err == nil {
		t.Fatal("expected error for wrong issuer")
	}
}

func TestRS256_WrongAudience_Rejected(t *testing.T) {
	f := newRSAFixture(t)
	defer f.close()
	v, err := sso.NewRS256Verifier(cfgFor(f.jwksURL), silentLogger())
	if err != nil {
		t.Fatalf("NewRS256Verifier: %v", err)
	}

	claims := validClaims()
	claims["aud"] = "https://other-api.test"
	tok := mintRS256(t, f.priv, rsaKid, claims)
	_, err = v.Verify(context.Background(), tok)
	if err == nil {
		t.Fatal("expected error for wrong audience")
	}
}

// TestRS256_AudienceAsArray exercises the RFC 7519 array-aud form. The token's
// aud is ["other", configured]; verification must succeed.
func TestRS256_AudienceAsArray(t *testing.T) {
	f := newRSAFixture(t)
	defer f.close()
	v, err := sso.NewRS256Verifier(cfgFor(f.jwksURL), silentLogger())
	if err != nil {
		t.Fatalf("NewRS256Verifier: %v", err)
	}

	claims := validClaims()
	claims["aud"] = []string{"https://other-api.test", "https://api.test"}
	tok := mintRS256(t, f.priv, rsaKid, claims)
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Errorf("expected success for array-aud containing configured audience; got %v", err)
	}
}

// TestRS256_AlgConfusion_Rejected is the load-bearing security test. An
// attacker mints an HS256 token using the RSA public key bytes as the HMAC
// secret. Without algorithm pinning, a naive verifier would accept this.
// With jwt.WithValidMethods([]string{"RS256"}), the RS256 verifier rejects it.
func TestRS256_AlgConfusion_Rejected(t *testing.T) {
	f := newRSAFixture(t)
	defer f.close()
	v, err := sso.NewRS256Verifier(cfgFor(f.jwksURL), silentLogger())
	if err != nil {
		t.Fatalf("NewRS256Verifier: %v", err)
	}

	// Mint an HS256 token where the HMAC secret is the RSA public modulus
	// bytes — the classic alg-confusion payload.
	hsClaims := validClaims()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, hsClaims)
	tok.Header["kid"] = rsaKid
	pubBytes := f.priv.N.Bytes()
	signed, err := tok.SignedString(pubBytes)
	if err != nil {
		t.Fatalf("HS256 mint: %v", err)
	}

	_, err = v.Verify(context.Background(), signed)
	if err == nil {
		t.Fatal("SECURITY: alg-confusion HS256 token was accepted by RS256 verifier")
	}
}

func TestRS256_ClaimMappingOverride(t *testing.T) {
	f := newRSAFixture(t)
	defer f.close()

	cfg := cfgFor(f.jwksURL)
	cfg.Claims = config.SSOClaimNames{UserID: "user_id", Email: "email_addr", DisplayName: "full_name"}

	v, err := sso.NewRS256Verifier(cfg, silentLogger())
	if err != nil {
		t.Fatalf("NewRS256Verifier: %v", err)
	}

	claims := validClaims()
	delete(claims, "sub")
	delete(claims, "email")
	delete(claims, "name")
	claims["user_id"] = "custom-user-42"
	claims["email_addr"] = "u42@example.com"
	claims["full_name"] = "User Forty-Two"
	claims["extra"] = "carry-into-custom"

	tok := mintRS256(t, f.priv, rsaKid, claims)
	got, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.UserID != "custom-user-42" {
		t.Errorf("UserID = %q; want custom-user-42", got.UserID)
	}
	if got.Email == nil || *got.Email != "u42@example.com" {
		t.Errorf("Email = %v; want u42@example.com", got.Email)
	}
	if got.DisplayName == nil || *got.DisplayName != "User Forty-Two" {
		t.Errorf("DisplayName = %v; want User Forty-Two", got.DisplayName)
	}
	if v, ok := got.Custom["extra"]; !ok || fmt.Sprint(v) != "carry-into-custom" {
		t.Errorf("Custom[extra] = %v; want carry-into-custom", got.Custom["extra"])
	}
}

func TestRS256_NotBeforeFuture_Rejected(t *testing.T) {
	f := newRSAFixture(t)
	defer f.close()
	v, err := sso.NewRS256Verifier(cfgFor(f.jwksURL), silentLogger())
	if err != nil {
		t.Fatalf("NewRS256Verifier: %v", err)
	}

	claims := validClaims()
	// 5 minutes in the future — well past the 30s clock skew.
	claims["nbf"] = time.Now().Add(5 * time.Minute).Unix()
	tok := mintRS256(t, f.priv, rsaKid, claims)
	_, err = v.Verify(context.Background(), tok)
	if err == nil {
		t.Fatal("expected error for nbf in the future")
	}
}

// TestRS256_JWKSUnreachable_Errors asserts that a JWKS URL that returns 500 at
// construction fails fast — not deferred to first user request.
func TestRS256_JWKSUnreachable_Errors(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()

	cfg := cfgFor(bad.URL + "/.well-known/jwks.json")
	_, err := sso.NewRS256Verifier(cfg, silentLogger())
	if err == nil {
		t.Fatal("expected NewRS256Verifier to fail when the JWKS URL is unreachable")
	}
}
