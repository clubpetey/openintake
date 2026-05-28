package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
)

// EmbeddedPublicKeyBase64 is the maintainer's Ed25519 public key (standard base64
// of the 32-byte key). It is filled during the keygen pause (phase final smoke
// step 1: `intake-license keygen` prints it). Empty in the committed source: a
// build with an empty key cannot verify a license, so Load treats a *present*
// license as an error until the maintainer embeds the real key.
const EmbeddedPublicKeyBase64 = "tw5CBPaQty7dhTa51G9JmC3h8EjCTipXy7eaLpusNKA="

// embeddedPublicKey decodes EmbeddedPublicKeyBase64.
//
//   - empty constant → (nil, nil)  — no embed; normal pre-keygen state
//   - non-empty + valid (correct base64, 32 bytes) → (key, nil)
//   - non-empty + invalid (bad base64 or wrong length) → (nil, error)
func embeddedPublicKey() (ed25519.PublicKey, error) {
	if EmbeddedPublicKeyBase64 == "" {
		return nil, nil
	}
	b, err := base64Decode(EmbeddedPublicKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("embedded public key is set but invalid: %w", err)
	}
	if len(b) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("embedded public key is set but invalid: decoded length %d, want %d", len(b), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(b), nil
}

func base64Decode(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }
