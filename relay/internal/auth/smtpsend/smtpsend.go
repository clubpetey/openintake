// Package smtpsend ships one-line auth-code emails to a configured SMTP server.
// The `Sender` interface is the test seam; `NetSMTP` is the production impl
// over stdlib net/smtp (SMTP-AUTH PLAIN/LOGIN over STARTTLS); `FakeSender` is
// the test double that captures (to, code) tuples in memory.
//
// Security: implementations MUST NOT include the SMTP password in any returned
// error. The stdlib net/smtp implementation does not leak the password on its
// own; this package adds no logging that could leak it.
package smtpsend

import (
	"context"
	"fmt"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
)

// Sender ships a one-line auth-code email to `to`.
type Sender interface {
	Send(ctx context.Context, to string, code string) error
}

// NetSMTP is the production net/smtp implementation. It uses smtp.PlainAuth
// over STARTTLS-capable servers (smtp.SendMail negotiates STARTTLS when the
// server advertises it on port 587/465). For port 25 plain delivery the
// password should be empty (no auth).
type NetSMTP struct {
	host string
	port int
	user string
	pass string
	from string
}

// NewNetSMTP constructs the production sender. The password is the RESOLVED
// secret value (caller passes it in via config.ResolveSecret); this package
// never reads the environment.
func NewNetSMTP(host string, port int, user, password, from string) *NetSMTP {
	return &NetSMTP{
		host: host,
		port: port,
		user: user,
		pass: password,
		from: from,
	}
}

// Send delivers a one-line auth-code email to `to`. The body is intentionally
// minimal — no HTML, no templating — to keep v0 deliverability simple.
// The error returned never contains the SMTP password.
func (n *NetSMTP) Send(ctx context.Context, to, code string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	addr := n.host + ":" + strconv.Itoa(n.port)

	subject := "Your intake verification code"
	body := "Your intake verification code is: " + code + "\r\n\r\n" +
		"This code expires in 10 minutes. If you did not request it, you can ignore this email.\r\n"

	msg := buildMessage(n.from, to, subject, body)

	var auth smtp.Auth
	if n.user != "" || n.pass != "" {
		auth = smtp.PlainAuth("", n.user, n.pass, n.host)
	}

	if err := smtp.SendMail(addr, auth, n.from, []string{to}, msg); err != nil {
		// stdlib's net/smtp errors do not embed the password — they carry the
		// SMTP server's response text and basic transport diagnostics. We
		// nonetheless wrap defensively without referencing n.pass.
		return fmt.Errorf("smtpsend: send to %s via %s: %w", to, addr, err)
	}
	return nil
}

// buildMessage produces a minimal RFC 5322 message. CRLF line endings per spec.
func buildMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: ")
	b.WriteString(from)
	b.WriteString("\r\n")
	b.WriteString("To: ")
	b.WriteString(to)
	b.WriteString("\r\n")
	b.WriteString("Subject: ")
	b.WriteString(subject)
	b.WriteString("\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

// SentRecord is one captured (to, code) pair from FakeSender.
type SentRecord struct {
	To   string
	Code string
}

// FakeSender is the in-memory test double — captures every Send call in order.
type FakeSender struct {
	mu   sync.Mutex
	sent []SentRecord
}

// NewFakeSender returns a fresh FakeSender.
func NewFakeSender() *FakeSender { return &FakeSender{} }

// Send records the (to, code) pair and returns nil.
func (f *FakeSender) Send(_ context.Context, to, code string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, SentRecord{To: to, Code: code})
	return nil
}

// Sent returns a copy of the captured records (ordered).
func (f *FakeSender) Sent() []SentRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]SentRecord, len(f.sent))
	copy(out, f.sent)
	return out
}
