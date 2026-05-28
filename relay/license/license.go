// Package license is the importable, dependency-free core of the license model:
// the License struct and the canonicalize/sign/verify primitives. It is shared
// verbatim by the relay (verifies with the embedded public key) and the
// maintainer-only intake-license CLI (signs with the private key, via a replace
// directive). Keeping ONE definition here is what guarantees the two modules agree
// byte-for-byte on what gets signed. No relay-internal imports may be added here.
package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const sigPrefix = "ed25519:"

// License is the signed entitlement (PROJECT.md §12). Field order is the canonical
// signing order; do not reorder without re-issuing every license.
type License struct {
	LicenseID    string    `json:"license_id"`
	IssuedTo     IssuedTo  `json:"issued_to"`
	Tier         string    `json:"tier"` // "pro" | "team" | "hosted"
	Adapters     []string  `json:"adapters"`
	MaxInstances int       `json:"max_relay_instances"`
	IssuedAt     time.Time `json:"issued_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	Signature    string    `json:"signature"`
}

// IssuedTo identifies the licensee.
type IssuedTo struct {
	Org   string `json:"org"`
	Email string `json:"email"`
}

// Canonicalize returns the exact bytes that are signed and verified: the JSON of
// the license with Signature cleared. Deterministic: Go marshals struct fields in
// declaration order and the License has no map fields. The argument is taken by
// value so the caller's Signature is never mutated.
func Canonicalize(l License) ([]byte, error) {
	l.Signature = ""
	b, err := json.Marshal(l)
	if err != nil {
		return nil, fmt.Errorf("license: canonicalize: %w", err)
	}
	return b, nil
}

// Sign sets l.Signature to "ed25519:" + base64(signature over Canonicalize(l)).
func Sign(priv ed25519.PrivateKey, l *License) error {
	canon, err := Canonicalize(*l)
	if err != nil {
		return err
	}
	sig := ed25519.Sign(priv, canon)
	l.Signature = sigPrefix + base64.StdEncoding.EncodeToString(sig)
	return nil
}

// Verify parses blob, checks the "ed25519:" signature against pub over the
// re-canonicalized license, and returns the parsed License on success. Any
// tampering (the canonical bytes no longer match the signature) fails.
func Verify(pub ed25519.PublicKey, blob []byte) (*License, error) {
	var l License
	if err := json.Unmarshal(blob, &l); err != nil {
		return nil, fmt.Errorf("license: parse: %w", err)
	}
	if !strings.HasPrefix(l.Signature, sigPrefix) {
		return nil, fmt.Errorf("license: signature missing %q prefix", sigPrefix)
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(l.Signature, sigPrefix))
	if err != nil {
		return nil, fmt.Errorf("license: signature not valid base64: %w", err)
	}
	canon, err := Canonicalize(l)
	if err != nil {
		return nil, err
	}
	if !ed25519.Verify(pub, canon, sig) {
		return nil, fmt.Errorf("license: signature verification failed (tampered or wrong key)")
	}
	return &l, nil
}
