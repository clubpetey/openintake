package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"intake/internal/auth"
	"intake/internal/auth/emailcode"
	"intake/internal/auth/emailjwt"
	"intake/internal/auth/smtpsend"
	"intake/internal/config"
	"intake/internal/server"
)

var testSecret = []byte("0123456789abcdef0123456789abcdef") // 32 bytes

func buildEmailServer(t *testing.T, fake smtpsend.Sender) (*server.Deps, http.Handler) {
	t.Helper()
	codes := emailcode.New(10*time.Minute, 10*time.Minute, 3, time.Now)
	emailSvc := server.NewEmailService(codes, fake, testSecret, 15*time.Minute)

	cfg := &config.Config{
		Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}},
		Auth: config.AuthConfig{
			Modes: config.AuthModes{Anonymous: true, Email: true},
			Email: config.EmailConfig{CodeTTL: "10m", JWTTTL: "15m"},
		},
	}
	deps := server.Deps{
		Auth:         auth.NewMiddleware(auth.NewStore(), &emailjwt.Verifier{Secret: testSecret}, nil),
		AuthCfg:      cfg.Auth,
		EmailService: emailSvc,
	}
	return &deps, server.New(cfg, deps)
}

func TestEmailStart_HappyPath(t *testing.T) {
	fake := smtpsend.NewFakeSender()
	_, mux := buildEmailServer(t, fake)

	body := bytes.NewBufferString(`{"email":"user@example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		MessageSent bool `json:"message_sent"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.MessageSent {
		t.Errorf("message_sent = false; want true")
	}
	sent := fake.Sent()
	if len(sent) != 1 {
		t.Fatalf("FakeSender captured %d; want 1", len(sent))
	}
	if sent[0].To != "user@example.com" {
		t.Errorf("captured To = %q; want user@example.com", sent[0].To)
	}
	if len(sent[0].Code) != 6 {
		t.Errorf("captured code %q is not 6 chars", sent[0].Code)
	}
}

func TestEmailStart_BadJSON_400(t *testing.T) {
	fake := smtpsend.NewFakeSender()
	_, mux := buildEmailServer(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", bytes.NewBufferString(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rr.Code)
	}
}

func TestEmailStart_InvalidEmail_400(t *testing.T) {
	fake := smtpsend.NewFakeSender()
	_, mux := buildEmailServer(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", bytes.NewBufferString(`{"email":"not-an-email"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rr.Code)
	}
}

func TestEmailStart_RateLimited_429_WithRetryAfter(t *testing.T) {
	fake := smtpsend.NewFakeSender()
	_, mux := buildEmailServer(t, fake)

	// Three within cap.
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", bytes.NewBufferString(`{"email":"user@example.com"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("issue %d: status = %d; body = %s", i, rr.Code, rr.Body.String())
		}
	}
	// Fourth exceeds cap.
	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", bytes.NewBufferString(`{"email":"user@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d; want 429", rr.Code)
	}
	retryAfter := rr.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Fatal("Retry-After header is empty; must be set on 429")
	}
	if n, err := strconv.Atoi(retryAfter); err != nil || n <= 0 {
		t.Errorf("Retry-After = %q; want positive integer seconds", retryAfter)
	}
	// Anti-enumeration: body must be the generic shape; must NOT include count or window-reset timestamp.
	body := rr.Body.String()
	if !strings.Contains(body, "rate_limited") {
		t.Errorf("429 body missing error.code rate_limited: %s", body)
	}
	if !strings.Contains(body, "too many codes requested for this email") {
		t.Errorf("429 body missing frozen-contract message text: %s", body)
	}
	if strings.Contains(body, "4") || strings.Contains(body, "count") || strings.Contains(body, "window") {
		t.Errorf("429 body must not expose count/window detail: %s", body)
	}
}

// failingSender always returns errSMTPFailure on Send. Used to drive the 502 path.
type failingSender struct{}

func (failingSender) Send(_ context.Context, _ string, _ string) error {
	return errSMTPFailure
}

var errSMTPFailure = errors.New("upstream rejected: 535 Authentication credentials invalid")

func TestEmailStart_SMTPFailure_502_NoDetailLeak(t *testing.T) {
	_, mux := buildEmailServer(t, failingSender{})

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", bytes.NewBufferString(`{"email":"user@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d; want 502", rr.Code)
	}
	body := rr.Body.String()
	// Body must NOT echo the SMTP detail.
	if strings.Contains(body, "535") || strings.Contains(body, "credentials") {
		t.Errorf("502 body leaked SMTP detail: %s", body)
	}
	if !strings.Contains(body, "smtp_error") {
		t.Errorf("502 body missing error.code smtp_error: %s", body)
	}
}

func TestEmailVerify_HappyPath_RoundTrips(t *testing.T) {
	fake := smtpsend.NewFakeSender()
	_, mux := buildEmailServer(t, fake)

	// 1. Start
	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", bytes.NewBufferString(`{"email":"user@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("start status %d; body %s", rr.Code, rr.Body.String())
	}
	code := fake.Sent()[0].Code

	// 2. Verify
	body := `{"email":"user@example.com","code":"` + code + `"}`
	req = httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("verify status %d; body %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
		User      struct {
			Email    string `json:"email"`
			Verified bool   `json:"verified"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("token is empty")
	}
	if resp.User.Email != "user@example.com" || !resp.User.Verified {
		t.Errorf("user = %+v; want {email:user@example.com, verified:true}", resp.User)
	}
	if !resp.ExpiresAt.After(time.Now()) {
		t.Errorf("expires_at %v must be in the future", resp.ExpiresAt)
	}

	// The token round-trips through emailjwt.Verify.
	email, err := emailjwt.Verify(testSecret, resp.Token)
	if err != nil {
		t.Fatalf("emailjwt.Verify(returned token): %v", err)
	}
	if email != "user@example.com" {
		t.Errorf("verified email = %q; want user@example.com", email)
	}
}

func TestEmailVerify_WrongCode_401_Generic(t *testing.T) {
	fake := smtpsend.NewFakeSender()
	_, mux := buildEmailServer(t, fake)

	// Start to ensure a code exists.
	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", bytes.NewBufferString(`{"email":"user@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("start: %d", rr.Code)
	}

	// Verify with the wrong code.
	body := `{"email":"user@example.com","code":"000000"}`
	req = httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", rr.Code)
	}
	// Generic body.
	respBody := rr.Body.String()
	if !strings.Contains(respBody, "invalid_code") {
		t.Errorf("body missing error.code invalid_code: %s", respBody)
	}
	// Must not say "not found" vs "expired" vs "used" — anti-enumeration.
	for _, leak := range []string{"not found", "expired", "already used", "consumed"} {
		if strings.Contains(strings.ToLower(respBody), leak) {
			t.Errorf("401 body leaks enumeration detail %q: %s", leak, respBody)
		}
	}
}

func TestEmailVerify_AlreadyUsed_401(t *testing.T) {
	fake := smtpsend.NewFakeSender()
	_, mux := buildEmailServer(t, fake)

	// Start.
	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", bytes.NewBufferString(`{"email":"user@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	code := fake.Sent()[0].Code

	// First verify succeeds.
	body := `{"email":"user@example.com","code":"` + code + `"}`
	req = httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("first verify: %d", rr.Code)
	}

	// Second verify of same code fails 401.
	req = httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("second verify status = %d; want 401", rr.Code)
	}
}

func TestEmailVerify_BadJSON_400(t *testing.T) {
	fake := smtpsend.NewFakeSender()
	_, mux := buildEmailServer(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/verify", bytes.NewBufferString(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rr.Code)
	}
}

// buildEmailServerNoAnon is like buildEmailServer but with Anonymous: false so
// the integration test exclusively exercises the JWT path (no false-pass via
// X-Intake-Session).
func buildEmailServerNoAnon(t *testing.T, fake smtpsend.Sender) (*server.Deps, http.Handler) {
	t.Helper()
	codes := emailcode.New(10*time.Minute, 10*time.Minute, 3, time.Now)
	emailSvc := server.NewEmailService(codes, fake, testSecret, 15*time.Minute)

	cfg := &config.Config{
		Server: config.ServerConfig{CORSOrigins: []string{"http://localhost:5173"}},
		Auth: config.AuthConfig{
			Modes: config.AuthModes{Anonymous: false, Email: true},
			Email: config.EmailConfig{CodeTTL: "10m", JWTTTL: "15m"},
		},
	}
	deps := server.Deps{
		Auth:         auth.NewMiddleware(auth.NewStore(), &emailjwt.Verifier{Secret: testSecret}, nil),
		AuthCfg:      cfg.Auth,
		EmailService: emailSvc,
	}
	return &deps, server.New(cfg, deps)
}

// Integration: full flow end-to-end — start → verify → spy endpoint behind
// deps.Auth.Handler asserts SessionContext.AuthMode=="email", Verified==true,
// and Email equals the normalized address.
func TestEmailFlow_DrivesTurnWithBearer(t *testing.T) {
	fake := smtpsend.NewFakeSender()
	deps, realMux := buildEmailServerNoAnon(t, fake)

	// Mount a test-only spy behind the auth middleware. The spy reads the
	// SessionContext and writes it as JSON so we can assert the fields.
	// We serve bootstrap routes via realMux and the spy directly.
	spyHandler := deps.Auth.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, _ := auth.FromContext(r.Context())
		type out struct {
			AuthMode string  `json:"auth_mode"`
			Verified bool    `json:"verified"`
			Email    *string `json:"email"`
		}
		var o out
		if sess != nil {
			o = out{AuthMode: sess.AuthMode, Verified: sess.Verified, Email: sess.Email}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(o)
	}))

	// Composite mux: spy on /_test_session, everything else to the real server.
	top := http.NewServeMux()
	top.Handle("GET /_test_session", spyHandler)
	top.Handle("/", realMux)

	// 1. start
	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", bytes.NewBufferString(`{"email":"user@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	top.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("start: %d body=%s", rr.Code, rr.Body.String())
	}
	code := fake.Sent()[0].Code

	// 2. verify
	vbody := `{"email":"user@example.com","code":"` + code + `"}`
	req = httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/verify", bytes.NewBufferString(vbody))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	top.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("verify: %d body=%s", rr.Code, rr.Body.String())
	}
	var verifyResp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &verifyResp); err != nil {
		t.Fatalf("decode verify: %v", err)
	}

	// 3. call the spy endpoint with the email JWT bearer.
	req = httptest.NewRequest(http.MethodGet, "/_test_session", nil)
	req.Header.Set("Authorization", "Bearer "+verifyResp.Token)
	rr = httptest.NewRecorder()
	top.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("spy: %d body=%s", rr.Code, rr.Body.String())
	}

	var sess struct {
		AuthMode string  `json:"auth_mode"`
		Verified bool    `json:"verified"`
		Email    *string `json:"email"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &sess); err != nil {
		t.Fatalf("decode spy: %v", err)
	}
	if sess.AuthMode != "email" {
		t.Errorf("AuthMode = %q; want \"email\"", sess.AuthMode)
	}
	if !sess.Verified {
		t.Errorf("Verified = false; want true")
	}
	if sess.Email == nil || *sess.Email != "user@example.com" {
		t.Errorf("Email = %v; want \"user@example.com\"", sess.Email)
	}
}

// TestEmailStart_NormalizesDisplayName proves that a /start request with a
// display-name form email normalizes to the bare address before issuing.
func TestEmailStart_NormalizesDisplayName(t *testing.T) {
	fake := smtpsend.NewFakeSender()
	_, mux := buildEmailServer(t, fake)

	body := bytes.NewBufferString(`{"email":"Alice Smith <alice@example.com>"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rr.Code, rr.Body.String())
	}
	sent := fake.Sent()
	if len(sent) != 1 {
		t.Fatalf("FakeSender captured %d; want 1", len(sent))
	}
	// Must be the bare address, not the display-name form.
	if sent[0].To != "alice@example.com" {
		t.Errorf("To = %q; want \"alice@example.com\"", sent[0].To)
	}
}

// TestEmailVerify_NormalizesDisplayName proves that issuing for a bare address
// then verifying with the display-name form succeeds (they normalize to the
// same key).
func TestEmailVerify_NormalizesDisplayName(t *testing.T) {
	fake := smtpsend.NewFakeSender()
	_, mux := buildEmailServer(t, fake)

	// Start with bare address.
	req := httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/start",
		bytes.NewBufferString(`{"email":"a@b.example"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("start status %d; body %s", rr.Code, rr.Body.String())
	}
	code := fake.Sent()[0].Code

	// Verify with display-name form — must succeed.
	verifyBody := `{"email":"A <a@b.example>","code":"` + code + `"}`
	req = httptest.NewRequest(http.MethodPost, "/v1/intake/auth/email/verify",
		bytes.NewBufferString(verifyBody))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("verify status %d; body %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		User struct {
			Email string `json:"email"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// user.email in the response must be the normalized bare address.
	if resp.User.Email != "a@b.example" {
		t.Errorf("user.email = %q; want \"a@b.example\"", resp.User.Email)
	}
}
