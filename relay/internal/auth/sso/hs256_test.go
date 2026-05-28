package sso_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"intake/internal/auth/sso"
	"intake/internal/config"
)

// hs256Secret is a deterministic 32-byte secret used in all HS256 tests.
var hs256Secret = []byte("0123456789abcdef0123456789abcdef")

// cfgForHS256 returns an SSOConfig that selects the HS256 path (JWKSURL empty).
func cfgForHS256() config.SSOConfig {
	return config.SSOConfig{
		Issuer:         "https://issuer.test/",
		Audience:       "https://api.test",
		HS256SecretEnv: "INTAKE_SSO_HS256_SECRET",
		Claims:         config.SSOClaimNames{UserID: "sub", Email: "email", DisplayName: "name"},
	}
}

// mintHS256 signs claims with the given secret.
func mintHS256(t *testing.T, secret []byte, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString(secret)
	if err != nil {
		t.Fatalf("HS256 SignedString: %v", err)
	}
	return s
}

func TestHS256_HappyPath(t *testing.T) {
	v, err := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)
	if err != nil {
		t.Fatalf("NewHS256Verifier: %v", err)
	}
	tok := mintHS256(t, hs256Secret, validClaims())

	got, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.UserID != "user-abc-123" {
		t.Errorf("UserID = %q; want user-abc-123", got.UserID)
	}
	if got.Email == nil || *got.Email != "user@example.com" {
		t.Errorf("Email = %v", got.Email)
	}
}

func TestHS256_TamperedToken_Rejected(t *testing.T) {
	v, _ := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)
	tok := mintHS256(t, hs256Secret, validClaims())
	tampered := []byte(tok)
	for i := range tampered {
		if tampered[i] == '.' && i+1 < len(tampered) {
			if tampered[i+1] == 'A' {
				tampered[i+1] = 'B'
			} else {
				tampered[i+1] = 'A'
			}
			break
		}
	}
	if _, err := v.Verify(context.Background(), string(tampered)); err == nil {
		t.Fatal("expected error for tampered token")
	}
}

func TestHS256_WrongSecret_Rejected(t *testing.T) {
	v, _ := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)
	other := []byte("ffffffffffffffffffffffffffffffff")
	tok := mintHS256(t, other, validClaims())
	if _, err := v.Verify(context.Background(), tok); err == nil {
		t.Fatal("expected error for token signed with a different secret")
	}
}

func TestHS256_Expired_Rejected(t *testing.T) {
	v, _ := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)
	claims := validClaims()
	claims["exp"] = time.Now().Add(-1 * time.Hour).Unix()
	tok := mintHS256(t, hs256Secret, claims)
	if _, err := v.Verify(context.Background(), tok); err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestHS256_WrongIssuer_Rejected(t *testing.T) {
	v, _ := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)
	claims := validClaims()
	claims["iss"] = "https://attacker.test/"
	tok := mintHS256(t, hs256Secret, claims)
	if _, err := v.Verify(context.Background(), tok); err == nil {
		t.Fatal("expected error for wrong issuer")
	}
}

func TestHS256_WrongAudience_Rejected(t *testing.T) {
	v, _ := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)
	claims := validClaims()
	claims["aud"] = "https://other-api.test"
	tok := mintHS256(t, hs256Secret, claims)
	if _, err := v.Verify(context.Background(), tok); err == nil {
		t.Fatal("expected error for wrong audience")
	}
}

// TestHS256_AlgConfusion_Rejected: an RS256 token must NOT be accepted by the
// HS256 verifier even if some attacker presents one. WithValidMethods enforces
// the alg whitelist.
func TestHS256_AlgConfusion_Rejected(t *testing.T) {
	v, _ := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, validClaims())
	signed, err := tok.SignedString(priv)
	if err != nil {
		t.Fatalf("RS256 SignedString: %v", err)
	}

	if _, err := v.Verify(context.Background(), signed); err == nil {
		t.Fatal("SECURITY: RS256 token accepted by HS256 verifier")
	}
}

func TestHS256_ClaimMappingOverride(t *testing.T) {
	cfg := cfgForHS256()
	cfg.Claims = config.SSOClaimNames{UserID: "user_id", Email: "email_addr", DisplayName: "full_name"}
	v, _ := sso.NewHS256Verifier(cfg, hs256Secret)

	claims := validClaims()
	delete(claims, "sub")
	delete(claims, "email")
	delete(claims, "name")
	claims["user_id"] = "hs-user-7"
	claims["email_addr"] = "h7@example.com"
	claims["full_name"] = "HS User Seven"

	tok := mintHS256(t, hs256Secret, claims)
	got, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.UserID != "hs-user-7" {
		t.Errorf("UserID = %q; want hs-user-7", got.UserID)
	}
	if got.Email == nil || *got.Email != "h7@example.com" {
		t.Errorf("Email = %v", got.Email)
	}
	if got.DisplayName == nil || *got.DisplayName != "HS User Seven" {
		t.Errorf("DisplayName = %v", got.DisplayName)
	}
}

func TestHS256_NotBeforeFuture_Rejected(t *testing.T) {
	v, _ := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)
	claims := validClaims()
	claims["nbf"] = time.Now().Add(5 * time.Minute).Unix()
	tok := mintHS256(t, hs256Secret, claims)
	if _, err := v.Verify(context.Background(), tok); err == nil {
		t.Fatal("expected error for nbf in the future")
	}
}

// -- clock-skew boundary tests --

func TestHS256_ExpiredWithinSkew_Accepted(t *testing.T) {
	// Token expired 15 seconds ago — should be accepted under the 30s skew.
	v, err := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)
	if err != nil {
		t.Fatalf("NewHS256Verifier: %v", err)
	}
	tok := mintHS256(t, hs256Secret, jwt.MapClaims{
		"iss": "https://issuer.test/",
		"aud": "https://api.test",
		"sub": "user-001",
		"iat": time.Now().Add(-1 * time.Hour).Unix(),
		"exp": time.Now().Add(-15 * time.Second).Unix(),
	})
	claims, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("token within skew window should be accepted; got %v", err)
	}
	if claims.UserID != "user-001" {
		t.Errorf("UserID = %q; want user-001", claims.UserID)
	}
}

func TestHS256_ExpiredBeyondSkew_Rejected(t *testing.T) {
	// Token expired 45 seconds ago — should be rejected (beyond 30s skew).
	v, _ := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)
	tok := mintHS256(t, hs256Secret, jwt.MapClaims{
		"iss": "https://issuer.test/",
		"aud": "https://api.test",
		"sub": "user-001",
		"iat": time.Now().Add(-1 * time.Hour).Unix(),
		"exp": time.Now().Add(-45 * time.Second).Unix(),
	})
	if _, err := v.Verify(context.Background(), tok); err == nil {
		t.Fatal("token 45s expired should be rejected (beyond 30s skew)")
	}
}

func TestHS256_NotBeforeWithinSkew_Accepted(t *testing.T) {
	// Token nbf 15 seconds in the future — should be accepted under skew.
	v, _ := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)
	tok := mintHS256(t, hs256Secret, jwt.MapClaims{
		"iss": "https://issuer.test/",
		"aud": "https://api.test",
		"sub": "user-001",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(15 * time.Minute).Unix(),
		"nbf": time.Now().Add(15 * time.Second).Unix(),
	})
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatalf("nbf within skew should be accepted; got %v", err)
	}
}

func TestHS256_NotBeforeBeyondSkew_Rejected(t *testing.T) {
	// Token nbf 45 seconds in the future — should be rejected.
	v, _ := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)
	tok := mintHS256(t, hs256Secret, jwt.MapClaims{
		"iss": "https://issuer.test/",
		"aud": "https://api.test",
		"sub": "user-001",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(15 * time.Minute).Unix(),
		"nbf": time.Now().Add(45 * time.Second).Unix(),
	})
	if _, err := v.Verify(context.Background(), tok); err == nil {
		t.Fatal("nbf 45s in future should be rejected (beyond 30s skew)")
	}
}

// -- M2M shape test --

func TestHS256_M2MShape_OnlySubPopulated(t *testing.T) {
	v, _ := sso.NewHS256Verifier(cfgForHS256(), hs256Secret)
	tok := mintHS256(t, hs256Secret, jwt.MapClaims{
		"iss": "https://issuer.test/",
		"aud": "https://api.test",
		"sub": "m2m-client@clients",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(15 * time.Minute).Unix(),
		// no "email", no "name"
	})
	claims, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("M2M-shape token should verify; got %v", err)
	}
	if claims.UserID != "m2m-client@clients" {
		t.Errorf("UserID = %q; want m2m-client@clients", claims.UserID)
	}
	if claims.Email != nil {
		t.Errorf("Email = %v; want nil for M2M-shape token", *claims.Email)
	}
	if claims.DisplayName != nil {
		t.Errorf("DisplayName = %v; want nil for M2M-shape token", *claims.DisplayName)
	}
}

// TestHS256_SecretTooShort_Rejected asserts the constructor enforces the
// 32-byte minimum on the resolved secret.
func TestHS256_SecretTooShort_Rejected(t *testing.T) {
	_, err := sso.NewHS256Verifier(cfgForHS256(), []byte("short"))
	if err == nil {
		t.Fatal("expected error for HS256 secret shorter than 32 bytes")
	}
	if !strings.Contains(err.Error(), "32") {
		t.Errorf("error should mention the 32-byte minimum; got %v", err)
	}
}
