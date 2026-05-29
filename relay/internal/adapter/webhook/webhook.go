package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"intake/internal/adapter"
	"intake/internal/payload"

	"github.com/google/uuid"
)

const defaultMaxAttempts = 3
const defaultBackoff = "exponential"

// Adapter POSTs the canonical payload JSON to a configured URL with
// configured headers, retrying on 5xx responses or network errors.
type Adapter struct {
	url         string
	headers     map[string]string
	maxAttempts int
	backoff     string // "exponential" | "fixed"
	client      *http.Client
}

// New returns an unconfigured Adapter. Call Configure before use.
func New() *Adapter {
	return &Adapter{
		maxAttempts: defaultMaxAttempts,
		backoff:     defaultBackoff,
		client:      &http.Client{Timeout: 15 * time.Second},
	}
}

func (a *Adapter) Name() string { return "webhook" }

func (a *Adapter) RequiresLicense() bool { return false }

// Capabilities advertises the accepted attachment MIME types for /init
// capability discovery (Phase 6, 6-i). In v0 webhook is a pass-through, so
// it accepts every type the relay-wide allowlist permits.
func (a *Adapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{
		AcceptedMIMETypes: []string{"image/png", "image/jpeg", "image/webp"},
	}
}

// Configure reads url, headers, retry.max_attempts, retry.backoff from the map.
// Keys match config.WebhookConfig yaml tags lowercased. Only url is required.
func (a *Adapter) Configure(cfg map[string]any) error {
	urlVal, ok := cfg["url"]
	if !ok {
		return fmt.Errorf("webhook: missing required config key 'url'")
	}
	urlStr, ok := urlVal.(string)
	if !ok || urlStr == "" {
		return fmt.Errorf("webhook: config key 'url' must be a non-empty string")
	}
	a.url = urlStr

	if h, ok := cfg["headers"]; ok {
		switch v := h.(type) {
		case map[string]string:
			a.headers = v
		case map[string]any:
			a.headers = make(map[string]string, len(v))
			for k, val := range v {
				s, ok := val.(string)
				if !ok {
					return fmt.Errorf("webhook: header value for %q must be a string", k)
				}
				a.headers[k] = s
			}
		}
	}

	if retry, ok := cfg["retry"]; ok {
		if rm, ok := retry.(map[string]any); ok {
			if ma, ok := rm["max_attempts"]; ok {
				switch v := ma.(type) {
				case int:
					a.maxAttempts = v
				case float64:
					a.maxAttempts = int(v)
				}
			}
			if b, ok := rm["backoff"]; ok {
				if bs, ok := b.(string); ok && bs != "" {
					if bs != "exponential" && bs != "fixed" {
						return fmt.Errorf("webhook: invalid retry.backoff %q (must be \"exponential\" or \"fixed\")", bs)
					}
					a.backoff = bs
				}
			}
		}
	}

	return nil
}

// Create marshals p to JSON and POSTs to the configured URL, retrying on
// 5xx or network errors. On success it returns a CreateResult; ExternalID
// is sourced from the response body field "external_id" if present, otherwise
// a new UUID is generated.
func (a *Adapter) Create(ctx context.Context, p *payload.IntakePayload) (*adapter.CreateResult, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("webhook: marshal payload: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < a.maxAttempts; attempt++ {
		if attempt > 0 {
			wait := a.backoffDuration(attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}

		result, retry, err := a.doRequest(ctx, body)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !retry {
			break
		}
	}
	return nil, fmt.Errorf("webhook: after %d attempts: %w", a.maxAttempts, lastErr)
}

// doRequest performs one HTTP POST. Returns (result, shouldRetry, error).
func (a *Adapter) doRequest(ctx context.Context, body []byte) (*adapter.CreateResult, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range a.headers {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		// Network error: retryable.
		return nil, true, fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 500 {
		// 5xx: retryable.
		return nil, true, fmt.Errorf("upstream returned %d: %s", resp.StatusCode, adapter.Truncate(string(respBody), 200))
	}
	if resp.StatusCode >= 400 {
		// 4xx: not retryable (client error, misconfiguration).
		return nil, false, fmt.Errorf("upstream returned %d: %s", resp.StatusCode, adapter.Truncate(string(respBody), 200))
	}

	// 2xx: success.
	externalID := extractExternalID(respBody)
	if externalID == "" {
		externalID = uuid.NewString()
	}
	return &adapter.CreateResult{
		ExternalID:  externalID,
		ExternalURL: "",
		AdapterName: "webhook",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}, false, nil
}

// backoffDuration returns the wait before attempt n (1-indexed: first retry = n=1).
func (a *Adapter) backoffDuration(n int) time.Duration {
	if a.backoff == "exponential" {
		// 500ms * 2^(n-1): 500ms, 1s, 2s, ...
		ms := 500.0 * math.Pow(2, float64(n-1))
		return time.Duration(ms) * time.Millisecond
	}
	// fixed: 500ms
	return 500 * time.Millisecond
}

// extractExternalID attempts to parse {"external_id":"..."} from the response body.
func extractExternalID(body []byte) string {
	var resp struct {
		ExternalID string `json:"external_id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return ""
	}
	return resp.ExternalID
}

// HealthCheck sends a HEAD request (falls back to GET if HEAD is unsupported).
// A missing or unreachable URL is an error; a non-5xx response (including 404)
// is considered reachable and returns nil.
func (a *Adapter) HealthCheck(ctx context.Context) error {
	if a.url == "" {
		return fmt.Errorf("webhook: not configured (url is empty)")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, a.url, nil)
	if err != nil {
		return fmt.Errorf("webhook health: build request: %w", err)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook health: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("webhook health: upstream returned %d", resp.StatusCode)
	}
	return nil
}

