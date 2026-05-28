package license

import (
	"crypto/ed25519"
	"encoding/base64"
)

// EmbeddedPublicKeyBase64 is the maintainer's Ed25519 public key (standard base64
// of the 32-byte key). It is filled during the keygen pause (phase final smoke
// step 1: `intake-license keygen` prints it). Empty in the committed source: a
// build with an empty key cannot verify a license, so Load treats a *present*
// license as an error until the maintainer embeds the real key.
const EmbeddedPublicKeyBase64 = ""

// embeddedPublicKey decodes EmbeddedPublicKeyBase64, or returns nil if unset/invalid.
func embeddedPublicKey() ed25519.PublicKey {
	if EmbeddedPublicKeyBase64 == "" {
		return nil
	}
	b, err := base64Decode(EmbeddedPublicKeyBase64)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil
	}
	return ed25519.PublicKey(b)
}

func base64Decode(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }
