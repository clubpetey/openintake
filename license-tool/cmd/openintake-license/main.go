// Command openintake-license is the MAINTAINER-ONLY license tool. It is NOT distributed
// (Q10): excluded from all release artifacts (goreleaser ignore / not published).
// It signs licenses with the maintainer's offline Ed25519 private key, using the
// SAME relay/license canonicalization the relay verifies (shared via replace).
//
// Building or running this tool does NOT let you issue valid licenses: every license
// must be signed with the maintainer's Ed25519 private key, which is never committed
// to this repository. Security rests on the secrecy of that key, not on this source.
//
//	openintake-license keygen [--key priv.key] [--pub pub.txt]
//	openintake-license sign   --in template.json --key priv.key [--out license.json] [--days 365]
//	openintake-license verify --in license.json --pubkey <base64>
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/clubpetey/openintake/relay/license"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		keyPath, pubPath := "openintake-license-private.key", "openintake-license-public.txt"
		parseFlags(os.Args[2:], map[string]*string{"key": &keyPath, "pub": &pubPath})
		err = doKeygen(keyPath, pubPath, os.Stdout)
	case "sign":
		in, key, out, days := "", "", "license.json", "365"
		parseFlags(os.Args[2:], map[string]*string{"in": &in, "key": &key, "out": &out, "days": &days})
		if in == "" || key == "" {
			err = fmt.Errorf("sign: --in and --key are required")
			break
		}
		d, perr := strconv.Atoi(days)
		if perr != nil {
			err = fmt.Errorf("sign: --days must be an integer: %w", perr)
			break
		}
		if d <= 0 {
			err = fmt.Errorf("sign: --days must be a positive integer, got %d", d)
			break
		}
		err = doSign(in, key, out, d, os.Stdout)
	case "verify":
		in, pubkey := "", ""
		parseFlags(os.Args[2:], map[string]*string{"in": &in, "pubkey": &pubkey})
		if in == "" || pubkey == "" {
			err = fmt.Errorf("verify: --in and --pubkey are required")
			break
		}
		err = doVerify(in, pubkey, os.Stdout)
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "openintake-license:", err)
		os.Exit(1)
	}
}

const usage = `openintake-license (maintainer-only)
  keygen [--key priv.key] [--pub pub.txt]
  sign   --in template.json --key priv.key [--out license.json] [--days 365]
  verify --in license.json --pubkey <base64>`

// parseFlags parses --name value and --name=value forms into dst.
func parseFlags(args []string, dst map[string]*string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) < 3 || a[:2] != "--" {
			continue
		}
		name := a[2:]
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			// --name=value form
			if p, ok := dst[name[:eq]]; ok {
				*p = name[eq+1:]
			}
			continue
		}
		// --name value form
		if i+1 < len(args) {
			if p, ok := dst[name]; ok {
				*p = args[i+1]
				i++
			}
		}
	}
}

// doKeygen generates an Ed25519 keypair, writes the private key (base64 of the
// 64-byte seed+pub) to keyPath (0600), writes the public key base64 to pubPath,
// and prints the public key base64 to out (for embedding in the relay).
func doKeygen(keyPath, pubPath string, out io.Writer) error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("keygen: %w", err)
	}
	privB64 := base64.StdEncoding.EncodeToString(priv)
	pubB64 := base64.StdEncoding.EncodeToString(pub)
	// Remove any pre-existing file so the fresh write always creates it at 0600.
	_ = os.Remove(keyPath)
	if err := os.WriteFile(keyPath, []byte(privB64), 0o600); err != nil {
		return fmt.Errorf("keygen: writing private key: %w", err)
	}
	if err := os.WriteFile(pubPath, []byte(pubB64), 0o644); err != nil {
		return fmt.Errorf("keygen: writing public key: %w", err)
	}
	fmt.Fprintf(out, "private key written to %s (KEEP OFFLINE — never commit)\n", keyPath)
	fmt.Fprintf(out, "public key written to %s\n", pubPath)
	fmt.Fprintf(out, "\nEmbed this public key in relay/internal/license/embedded_key.go:\n")
	fmt.Fprintf(out, "  const EmbeddedPublicKeyBase64 = %q\n", pubB64)
	return nil
}

// doSign reads a license template JSON, loads the base64 private key from keyPath,
// stamps issued_at=now and expires_at=now+days (UTC, second precision), signs with
// the shared relay/license.Sign, and writes the signed license to outPath.
// NOTE: sign ALWAYS overwrites any issued_at/expires_at present in the template JSON
// with freshly computed values (now and now+--days). There is no way to preserve
// or supply custom timestamps via the template; the stamp is authoritative.
func doSign(inPath, keyPath, outPath string, days int, out io.Writer) error {
	tmpl, err := os.ReadFile(inPath)
	if err != nil {
		return fmt.Errorf("sign: reading template %s: %w", inPath, err)
	}
	var lic license.License
	if err := json.Unmarshal(tmpl, &lic); err != nil {
		return fmt.Errorf("sign: parsing template: %w", err)
	}
	privB64, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("sign: reading private key %s: %w", keyPath, err)
	}
	privBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(privB64)))
	if err != nil {
		return fmt.Errorf("sign: private key not valid base64: %w", err)
	}
	if len(privBytes) != ed25519.PrivateKeySize {
		return fmt.Errorf("sign: private key wrong size (%d; want %d)", len(privBytes), ed25519.PrivateKeySize)
	}
	now := time.Now().UTC().Truncate(time.Second)
	lic.IssuedAt = now
	lic.ExpiresAt = now.Add(time.Duration(days) * 24 * time.Hour)
	if err := license.Sign(ed25519.PrivateKey(privBytes), &lic); err != nil {
		return fmt.Errorf("sign: %w", err)
	}
	signed, err := json.MarshalIndent(lic, "", "  ")
	if err != nil {
		return fmt.Errorf("sign: marshal: %w", err)
	}
	if err := os.WriteFile(outPath, signed, 0o644); err != nil {
		return fmt.Errorf("sign: writing %s: %w", outPath, err)
	}
	fmt.Fprintf(out, "signed license written to %s (tier=%s, adapters=%v, expires=%s)\n",
		outPath, lic.Tier, lic.Adapters, lic.ExpiresAt.Format("2006-01-02"))
	return nil
}

// doVerify checks a signed license file against a base64 public key.
func doVerify(inPath, pubB64 string, out io.Writer) error {
	pubBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(pubB64))
	if err != nil {
		return fmt.Errorf("verify: public key not valid base64: %w", err)
	}
	if len(pubBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("verify: public key wrong size (%d; want %d)", len(pubBytes), ed25519.PublicKeySize)
	}
	blob, err := os.ReadFile(inPath)
	if err != nil {
		return fmt.Errorf("verify: reading %s: %w", inPath, err)
	}
	lic, err := license.Verify(ed25519.PublicKey(pubBytes), blob)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	fmt.Fprintf(out, "OK — license %s, tier=%s, adapters=%v, expires=%s\n",
		lic.LicenseID, lic.Tier, lic.Adapters, lic.ExpiresAt.Format("2006-01-02"))
	return nil
}
