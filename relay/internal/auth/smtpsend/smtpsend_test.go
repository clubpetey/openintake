package smtpsend_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"intake/internal/auth/smtpsend"
)

func TestFakeSender_CapturesInOrder(t *testing.T) {
	f := smtpsend.NewFakeSender()
	ctx := context.Background()
	if err := f.Send(ctx, "alice@example.com", "111111"); err != nil {
		t.Fatalf("Send 1: %v", err)
	}
	if err := f.Send(ctx, "bob@example.com", "222222"); err != nil {
		t.Fatalf("Send 2: %v", err)
	}
	sent := f.Sent()
	if len(sent) != 2 {
		t.Fatalf("len(Sent) = %d; want 2", len(sent))
	}
	if sent[0].To != "alice@example.com" || sent[0].Code != "111111" {
		t.Errorf("Sent[0] = %+v; want {alice@example.com, 111111}", sent[0])
	}
	if sent[1].To != "bob@example.com" || sent[1].Code != "222222" {
		t.Errorf("Sent[1] = %+v; want {bob@example.com, 222222}", sent[1])
	}
}

func TestFakeSender_ThreadSafe(t *testing.T) {
	f := smtpsend.NewFakeSender()
	const N = 50
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = f.Send(context.Background(), "x@example.com", "123456")
		}()
	}
	wg.Wait()
	if got := len(f.Sent()); got != N {
		t.Errorf("len(Sent) = %d; want %d", got, N)
	}
}

func TestNewNetSMTP_StoresParams(t *testing.T) {
	// We can't dial without a real server here — but we CAN assert the
	// constructor returns a non-nil sender that satisfies the interface
	// and that subsequent Send returns an error (no SMTP server on 127.0.0.1:1).
	n := smtpsend.NewNetSMTP("127.0.0.1", 1, "user", "pass", "Intake <noreply@example.com>")
	if n == nil {
		t.Fatal("NewNetSMTP returned nil")
	}
	var _ smtpsend.Sender = n // compile-time interface check
	err := n.Send(context.Background(), "to@example.com", "123456")
	if err == nil {
		t.Fatal("Send against unreachable 127.0.0.1:1 must error")
	}
	// SECURITY: the error MUST NOT contain the password.
	if strings.Contains(strings.ToLower(err.Error()), strings.ToLower("pass")) {
		// note: literal token "pass" intentionally matches what we passed —
		// if the error embeds it, that is a leak.
		t.Errorf("Send error leaked password: %v", err)
	}
}
