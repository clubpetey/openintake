package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/clubpetey/openintake/relay/internal/auth"
	"github.com/clubpetey/openintake/relay/internal/budget"
	"github.com/clubpetey/openintake/relay/internal/llm"
)

// fakeProvider is a minimal llm.Provider that emits a single SSEDone with
// caller-controlled input/output token counts. Used by Phase 5 /turn tests
// so the budget Commit + Store RecordTurn paths can be exercised without
// hitting a real LLM.
type fakeProvider struct {
	inputTokens  int
	outputTokens int
}

func (p *fakeProvider) Name() string { return "fake" }
func (p *fakeProvider) Chat(ctx context.Context, msgs []llm.Message, opts llm.ChatOptions) (<-chan llm.ChatChunk, error) {
	ch := make(chan llm.ChatChunk, 1)
	ch <- llm.ChatChunk{Done: true, InputTokens: p.inputTokens, OutputTokens: p.outputTokens}
	close(ch)
	return ch, nil
}

func TestTurnHandler_SessionTurnsExhausted_Returns429(t *testing.T) {
	store := auth.NewStoreWithCaps(2, 0, time.Hour, time.Now)
	id := store.Issue()
	// Pre-fill turns to cap.
	store.RecordTurn(id, 0)
	store.RecordTurn(id, 0)

	deps := Deps{
		Auth:      auth.NewMiddleware(store, nil, nil),
		Provider:  &fakeProvider{inputTokens: 10, outputTokens: 10},
		Model:     "test",
		MaxTokens: 100,
	}
	h := turnHandler(deps)

	req := httptest.NewRequest("POST", "/v1/intake/turn", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	// Attach SessionContext via WithSession (bypass the middleware in this unit test).
	ctx := auth.WithSession(req.Context(), &auth.SessionContext{SessionID: id, AuthMode: "anonymous"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d; want 429", rec.Code)
	}
	var body ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "session_turns_exhausted" {
		t.Errorf("code = %q; want session_turns_exhausted", body.Error.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("Retry-After header missing on 429")
	}
}

func TestTurnHandler_SessionExpired_Returns401(t *testing.T) {
	store := auth.NewStoreWithCaps(20, 8000, time.Hour, time.Now)
	// SessionID that was never issued.
	id := "00000000-0000-0000-0000-000000000000"

	deps := Deps{
		Auth:      auth.NewMiddleware(store, nil, nil),
		Provider:  &fakeProvider{},
		Model:     "test",
		MaxTokens: 100,
	}
	h := turnHandler(deps)
	req := httptest.NewRequest("POST", "/v1/intake/turn", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	ctx := auth.WithSession(req.Context(), &auth.SessionContext{SessionID: id, AuthMode: "anonymous"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", rec.Code)
	}
	var body ErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Error.Code != "session_expired" {
		t.Errorf("code = %q; want session_expired", body.Error.Code)
	}
}

func TestTurnHandler_BudgetExhausted_Returns503(t *testing.T) {
	store := auth.NewStoreWithCaps(0, 0, 0, time.Now) // no session caps
	id := store.Issue()

	tracker := budget.New(100, 100, time.Now)
	tracker.Commit("", 95, 95) // near cap

	deps := Deps{
		Auth:      auth.NewMiddleware(store, nil, nil),
		Provider:  &fakeProvider{inputTokens: 10, outputTokens: 10},
		Model:     "test",
		MaxTokens: 100,
		Budget:    tracker,
	}
	h := turnHandler(deps)
	req := httptest.NewRequest("POST", "/v1/intake/turn", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	ctx := auth.WithSession(req.Context(), &auth.SessionContext{SessionID: id, AuthMode: "anonymous"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; want 503", rec.Code)
	}
	var body ErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Error.Code != "daily_budget_exhausted" {
		t.Errorf("code = %q; want daily_budget_exhausted", body.Error.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("Retry-After header missing on 503")
	}
}

func TestTurnHandler_CommitOnSSEDone_RecordsTokens(t *testing.T) {
	store := auth.NewStoreWithCaps(20, 8000, time.Hour, time.Now)
	id := store.Issue()
	tracker := budget.New(10000, 10000, time.Now)

	deps := Deps{
		Auth:      auth.NewMiddleware(store, nil, nil),
		Provider:  &fakeProvider{inputTokens: 50, outputTokens: 25},
		Model:     "test",
		MaxTokens: 100,
		Budget:    tracker,
	}
	h := turnHandler(deps)
	req := httptest.NewRequest("POST", "/v1/intake/turn", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	ctx := auth.WithSession(req.Context(), &auth.SessionContext{SessionID: id, AuthMode: "anonymous"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (SSE)", rec.Code)
	}
	// budget.Commit must have fired.
	in, out, _ := tracker.Snapshot("")
	if in != 50 || out != 25 {
		t.Errorf("budget after Commit = (%d,%d); want (50,25)", in, out)
	}
	// auth.Store.RecordTurn must have fired.
	ok, _, _ := store.CheckSession(id)
	if !ok {
		t.Fatal("post-Commit CheckSession rejected; should still pass (1 of 20 turns used)")
	}
	// One more validation: a second turn should also pass and increment.
	req2 := httptest.NewRequest("POST", "/v1/intake/turn", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	req2 = req2.WithContext(auth.WithSession(req2.Context(), &auth.SessionContext{SessionID: id, AuthMode: "anonymous"}))
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	in2, out2, _ := tracker.Snapshot("")
	if in2 != 100 || out2 != 50 {
		t.Errorf("budget after 2nd Commit = (%d,%d); want (100,50)", in2, out2)
	}
}

func TestTurnHandler_TenantKeyFromHeader(t *testing.T) {
	store := auth.NewStoreWithCaps(0, 0, 0, time.Now)
	id := store.Issue()
	tracker := budget.New(10000, 10000, time.Now)

	deps := Deps{
		Auth:      auth.NewMiddleware(store, nil, nil),
		Provider:  &fakeProvider{inputTokens: 10, outputTokens: 5},
		Model:     "test",
		MaxTokens: 100,
		Budget:    tracker,
	}
	h := turnHandler(deps)
	req := httptest.NewRequest("POST", "/v1/intake/turn", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("X-Intake-Tenant", "acme")
	ctx := auth.WithSession(req.Context(), &auth.SessionContext{SessionID: id, AuthMode: "anonymous"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	// Counters must land under tenant "acme", not the empty key.
	in, _, _ := tracker.Snapshot("acme")
	if in != 10 {
		t.Errorf("tenant acme input = %d; want 10", in)
	}
	inEmpty, _, _ := tracker.Snapshot("")
	if inEmpty != 0 {
		t.Errorf("empty tenant input = %d; want 0 (request had X-Intake-Tenant)", inEmpty)
	}
}
