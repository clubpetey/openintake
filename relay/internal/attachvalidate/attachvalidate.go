// Package attachvalidate decodes and validates inline attachments carried in
// a SubmitRequest. It is the single source of truth for Phase 6 magic-byte +
// size-cap enforcement; submitHandler calls ValidateAll between Builder.Build
// and Router.Route. Each adapter that needs raw bytes calls DecodeOne inside
// its own Create() — the validation already passed by then; this is the
// cheap (base64) decode helper.
//
// Errors are SENTINELS — never wrapped — so submitHandler can map them to
// specific HTTP status codes via errors.Is. The six exported errors cover
// every misuse path. The first-encountered error wins on a multi-attachment
// request; the rest are not inspected.
package attachvalidate

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"intake/internal/payload"
)

// Decoded is the validator's per-attachment output: bytes pulled out of the
// data: URL, with the magic-byte-detected MIME and the byte length. Not used
// in 6-i beyond returning a value to the caller; 6-ii's adapters will use
// the equivalent of this via DecodeOne inside their own Create().
type Decoded struct {
	Raw       []byte
	MIMEType  string
	SizeBytes int
	Label     string
	Type      payload.AttachmentType
}

// Config carries the validator's enforcement knobs. submitHandler composes
// this from deps.AttachmentsCfg + deps.AttachmentMIMEs (the
// capabilities-intersection list — so attachvalidate doesn't need to know
// about adapter capabilities directly).
type Config struct {
	MaxSizeBytes     int
	MaxTotalBytes    int
	AllowedMIMETypes []string
}

// Sentinel errors. submitHandler uses errors.Is to map each to a specific
// HTTP status (413/415/400). The package never wraps these; callers can rely
// on identity comparison.
var (
	ErrAttachmentTooLarge        = errors.New("attachment exceeds max_size_bytes")
	ErrAggregateTooLarge         = errors.New("attachments exceed total cap")
	ErrMIMENotAllowed            = errors.New("attachment mime_type not in allowlist")
	ErrMIMEMismatch              = errors.New("attachment bytes do not match declared mime_type")
	ErrBadDataURL                = errors.New("attachment url is not a valid data: URL")
	ErrAttachmentTypeUnsupported = errors.New("attachment type unsupported in v0")
)

// ValidateAll decodes and validates every attachment in atts. Returns the
// decoded slice (1:1 with atts) on success, or the FIRST encountered sentinel
// error on any failure. A nil/empty atts is a successful no-op.
//
// Validation order per attachment:
//  1. Type must be "screenshot" (v0 only — "file" rejected).
//  2. MIME must be in cfg.AllowedMIMETypes (an empty allowlist rejects all).
//  3. URL must be a "data:<mime>;base64,<payload>" with non-empty payload.
//  4. base64 must decode cleanly.
//  5. Decoded length must be <= cfg.MaxSizeBytes.
//  6. Sum of decoded lengths must be <= cfg.MaxTotalBytes.
//  7. net/http.DetectContentType on the first 512 bytes must match the
//     declared MIME (case-insensitive; ignores params like "; charset=").
func ValidateAll(atts []payload.Attachment, cfg Config) ([]Decoded, error) {
	if len(atts) == 0 {
		return nil, nil
	}
	out := make([]Decoded, 0, len(atts))
	allowed := buildAllowlistSet(cfg.AllowedMIMETypes)
	total := 0
	for _, a := range atts {
		// 1. type guard
		if a.Type != payload.AttachmentTypeScreenshot {
			return nil, ErrAttachmentTypeUnsupported
		}
		// 2. mime allowlist
		if _, ok := allowed[a.MimeType]; !ok {
			return nil, ErrMIMENotAllowed
		}
		// 3-4. parse + decode the data: URL
		raw, err := decodeDataURL(a.Url)
		if err != nil {
			return nil, err
		}
		// 5. per-attachment cap
		if cfg.MaxSizeBytes > 0 && len(raw) > cfg.MaxSizeBytes {
			return nil, ErrAttachmentTooLarge
		}
		// 6. aggregate cap
		total += len(raw)
		if cfg.MaxTotalBytes > 0 && total > cfg.MaxTotalBytes {
			return nil, ErrAggregateTooLarge
		}
		// 7. magic-byte match
		sniffed := sniffMIME(raw)
		if !mimeBaseEqual(sniffed, a.MimeType) {
			return nil, ErrMIMEMismatch
		}
		label := ""
		if a.Label != nil {
			label = *a.Label
		}
		out = append(out, Decoded{
			Raw:       raw,
			MIMEType:  a.MimeType,
			SizeBytes: len(raw),
			Label:     label,
			Type:      a.Type,
		})
	}
	return out, nil
}

// DecodeOne is the per-adapter helper. Decodes one Attachment's data: URL to
// raw bytes + the magic-byte-detected MIME. submitHandler has already passed
// the same attachment through ValidateAll by the time any adapter sees it;
// DecodeOne is for adapters that need the raw bytes for native upload
// (chatwoot inline base64, linear asset upload, zendesk uploads.json).
// Returns ErrBadDataURL on malformed input. Does NOT enforce caps or
// allowlist — those are submitHandler's job.
func DecodeOne(att payload.Attachment) (raw []byte, mime string, err error) {
	raw, err = decodeDataURL(att.Url)
	if err != nil {
		return nil, "", err
	}
	return raw, sniffMIME(raw), nil
}

// decodeDataURL parses "data:<mime>;base64,<payload>" and returns the raw
// bytes. Returns ErrBadDataURL on any malformed input.
func decodeDataURL(s string) ([]byte, error) {
	const prefix = "data:"
	const marker = ";base64,"
	if !strings.HasPrefix(s, prefix) {
		return nil, ErrBadDataURL
	}
	idx := strings.Index(s, marker)
	if idx < 0 {
		return nil, ErrBadDataURL
	}
	encoded := s[idx+len(marker):]
	if encoded == "" {
		return nil, ErrBadDataURL
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, ErrBadDataURL
	}
	if len(raw) == 0 {
		return nil, ErrBadDataURL
	}
	return raw, nil
}

// sniffMIME returns net/http.DetectContentType's verdict on the first 512
// bytes. Returns the lower-cased MIME (no params) so mimeBaseEqual can
// match it against the declared value.
func sniffMIME(raw []byte) string {
	limit := len(raw)
	if limit > 512 {
		limit = 512
	}
	full := http.DetectContentType(raw[:limit])
	// DetectContentType may append "; charset=...", "; boundary=..." etc.
	if i := strings.IndexByte(full, ';'); i >= 0 {
		full = full[:i]
	}
	return strings.ToLower(strings.TrimSpace(full))
}

// mimeBaseEqual compares two MIME values case-insensitively, ignoring
// params and whitespace.
func mimeBaseEqual(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func buildAllowlistSet(list []string) map[string]struct{} {
	out := make(map[string]struct{}, len(list))
	for _, m := range list {
		out[m] = struct{}{}
	}
	return out
}
