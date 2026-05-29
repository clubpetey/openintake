package attachvalidate_test

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"intake/internal/attachvalidate"
	"intake/internal/payload"
)

func defaultCfg() attachvalidate.Config {
	return attachvalidate.Config{
		MaxSizeBytes:     5_242_880,
		MaxTotalBytes:    10_485_760,
		AllowedMIMETypes: []string{"image/png", "image/jpeg", "image/webp"},
	}
}

func att(mime, dataURL string) payload.Attachment {
	return payload.Attachment{
		Type:      payload.AttachmentTypeScreenshot,
		MimeType:  mime,
		Url:       dataURL,
		SizeBytes: 0, // declared size_bytes is not the validation source of truth
	}
}

func dataURL(mime string, raw []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)
}

func TestValidateAll_GoldenPNG_OK(t *testing.T) {
	a := att("image/png", dataURL("image/png", attachvalidate.GoldenPNG()))
	decoded, err := attachvalidate.ValidateAll([]payload.Attachment{a}, defaultCfg())
	if err != nil {
		t.Fatalf("ValidateAll: %v", err)
	}
	if len(decoded) != 1 {
		t.Fatalf("decoded len = %d; want 1", len(decoded))
	}
	if decoded[0].MIMEType != "image/png" {
		t.Errorf("MIMEType = %q; want image/png", decoded[0].MIMEType)
	}
	if decoded[0].SizeBytes != len(attachvalidate.GoldenPNG()) {
		t.Errorf("SizeBytes = %d; want %d", decoded[0].SizeBytes, len(attachvalidate.GoldenPNG()))
	}
}

func TestValidateAll_GoldenJPEG_OK(t *testing.T) {
	a := att("image/jpeg", dataURL("image/jpeg", attachvalidate.GoldenJPEG()))
	if _, err := attachvalidate.ValidateAll([]payload.Attachment{a}, defaultCfg()); err != nil {
		t.Fatalf("ValidateAll JPEG: %v", err)
	}
}

func TestValidateAll_GoldenWebP_OK(t *testing.T) {
	a := att("image/webp", dataURL("image/webp", attachvalidate.GoldenWebP()))
	if _, err := attachvalidate.ValidateAll([]payload.Attachment{a}, defaultCfg()); err != nil {
		t.Fatalf("ValidateAll WebP: %v", err)
	}
}

func TestValidateAll_MIMEMismatch_PNGBytesLabeledJPEG(t *testing.T) {
	a := att("image/jpeg", dataURL("image/jpeg", attachvalidate.GoldenPNG())) // declared JPEG, actually PNG
	_, err := attachvalidate.ValidateAll([]payload.Attachment{a}, defaultCfg())
	if !errors.Is(err, attachvalidate.ErrMIMEMismatch) {
		t.Errorf("err = %v; want ErrMIMEMismatch", err)
	}
}

func TestValidateAll_MIMENotAllowed(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowedMIMETypes = []string{"image/jpeg"} // PNG not in this allowlist
	a := att("image/png", dataURL("image/png", attachvalidate.GoldenPNG()))
	_, err := attachvalidate.ValidateAll([]payload.Attachment{a}, cfg)
	if !errors.Is(err, attachvalidate.ErrMIMENotAllowed) {
		t.Errorf("err = %v; want ErrMIMENotAllowed", err)
	}
}

func TestValidateAll_EmptyAllowlist_RejectsEverything(t *testing.T) {
	cfg := defaultCfg()
	cfg.AllowedMIMETypes = []string{}
	a := att("image/png", dataURL("image/png", attachvalidate.GoldenPNG()))
	_, err := attachvalidate.ValidateAll([]payload.Attachment{a}, cfg)
	if !errors.Is(err, attachvalidate.ErrMIMENotAllowed) {
		t.Errorf("err = %v; want ErrMIMENotAllowed (empty allowlist case)", err)
	}
}

func TestValidateAll_AttachmentTooLarge(t *testing.T) {
	cfg := defaultCfg()
	cfg.MaxSizeBytes = 10 // tiny
	a := att("image/png", dataURL("image/png", attachvalidate.GoldenPNG()))
	_, err := attachvalidate.ValidateAll([]payload.Attachment{a}, cfg)
	if !errors.Is(err, attachvalidate.ErrAttachmentTooLarge) {
		t.Errorf("err = %v; want ErrAttachmentTooLarge", err)
	}
}

// L017 boundary: <= must allow exactly equal sizes.
func TestValidateAll_AttachmentSizeBoundaryEqual_OK(t *testing.T) {
	cfg := defaultCfg()
	cfg.MaxSizeBytes = len(attachvalidate.GoldenPNG())
	a := att("image/png", dataURL("image/png", attachvalidate.GoldenPNG()))
	if _, err := attachvalidate.ValidateAll([]payload.Attachment{a}, cfg); err != nil {
		t.Errorf("ValidateAll at size boundary equal: %v; want OK", err)
	}
}

func TestValidateAll_AggregateTooLarge(t *testing.T) {
	cfg := defaultCfg()
	cfg.MaxTotalBytes = len(attachvalidate.GoldenPNG()) // one fits, two does not
	a := att("image/png", dataURL("image/png", attachvalidate.GoldenPNG()))
	_, err := attachvalidate.ValidateAll([]payload.Attachment{a, a}, cfg)
	if !errors.Is(err, attachvalidate.ErrAggregateTooLarge) {
		t.Errorf("err = %v; want ErrAggregateTooLarge", err)
	}
}

func TestValidateAll_AggregateBoundaryEqual_OK(t *testing.T) {
	cfg := defaultCfg()
	cfg.MaxTotalBytes = 2 * len(attachvalidate.GoldenPNG())
	a := att("image/png", dataURL("image/png", attachvalidate.GoldenPNG()))
	if _, err := attachvalidate.ValidateAll([]payload.Attachment{a, a}, cfg); err != nil {
		t.Errorf("ValidateAll at aggregate boundary equal: %v; want OK", err)
	}
}

func TestValidateAll_BadDataURL_NotDataURL(t *testing.T) {
	a := att("image/png", "http://example.com/foo.png")
	_, err := attachvalidate.ValidateAll([]payload.Attachment{a}, defaultCfg())
	if !errors.Is(err, attachvalidate.ErrBadDataURL) {
		t.Errorf("err = %v; want ErrBadDataURL", err)
	}
}

func TestValidateAll_BadDataURL_MissingBase64Marker(t *testing.T) {
	a := att("image/png", "data:image/png,iVBORw0KGgo")
	_, err := attachvalidate.ValidateAll([]payload.Attachment{a}, defaultCfg())
	if !errors.Is(err, attachvalidate.ErrBadDataURL) {
		t.Errorf("err = %v; want ErrBadDataURL", err)
	}
}

func TestValidateAll_BadDataURL_CorruptBase64(t *testing.T) {
	a := att("image/png", "data:image/png;base64,!!!not-base64!!!")
	_, err := attachvalidate.ValidateAll([]payload.Attachment{a}, defaultCfg())
	if !errors.Is(err, attachvalidate.ErrBadDataURL) {
		t.Errorf("err = %v; want ErrBadDataURL", err)
	}
}

func TestValidateAll_BadDataURL_EmptyData(t *testing.T) {
	a := att("image/png", "data:image/png;base64,")
	_, err := attachvalidate.ValidateAll([]payload.Attachment{a}, defaultCfg())
	if !errors.Is(err, attachvalidate.ErrBadDataURL) {
		t.Errorf("err = %v; want ErrBadDataURL", err)
	}
}

func TestValidateAll_TypeFileRejected(t *testing.T) {
	a := payload.Attachment{
		Type:     payload.AttachmentTypeFile,
		MimeType: "image/png",
		Url:      dataURL("image/png", attachvalidate.GoldenPNG()),
	}
	_, err := attachvalidate.ValidateAll([]payload.Attachment{a}, defaultCfg())
	if !errors.Is(err, attachvalidate.ErrAttachmentTypeUnsupported) {
		t.Errorf("err = %v; want ErrAttachmentTypeUnsupported", err)
	}
}

func TestValidateAll_EmptySliceOK(t *testing.T) {
	decoded, err := attachvalidate.ValidateAll(nil, defaultCfg())
	if err != nil {
		t.Errorf("ValidateAll on nil: %v; want nil", err)
	}
	if len(decoded) != 0 {
		t.Errorf("decoded len = %d; want 0", len(decoded))
	}
}

func TestDecodeOne_OK(t *testing.T) {
	a := att("image/png", dataURL("image/png", attachvalidate.GoldenPNG()))
	raw, mime, err := attachvalidate.DecodeOne(a)
	if err != nil {
		t.Fatalf("DecodeOne: %v", err)
	}
	if mime != "image/png" {
		t.Errorf("mime = %q; want image/png", mime)
	}
	if string(raw) != string(attachvalidate.GoldenPNG()) {
		t.Errorf("raw bytes mismatch")
	}
}

func TestDecodeOne_BadDataURL(t *testing.T) {
	a := att("image/png", "not-a-data-url")
	_, _, err := attachvalidate.DecodeOne(a)
	if !errors.Is(err, attachvalidate.ErrBadDataURL) {
		t.Errorf("err = %v; want ErrBadDataURL", err)
	}
}

// Sentinel-error sanity: each exported error has a stable, descriptive message.
func TestSentinelErrorMessages(t *testing.T) {
	cases := map[error]string{
		attachvalidate.ErrAttachmentTooLarge:        "max_size_bytes",
		attachvalidate.ErrAggregateTooLarge:         "total cap",
		attachvalidate.ErrMIMENotAllowed:            "allowlist",
		attachvalidate.ErrMIMEMismatch:              "match",
		attachvalidate.ErrBadDataURL:                "data:",
		attachvalidate.ErrAttachmentTypeUnsupported: "v0",
	}
	for err, sub := range cases {
		if !strings.Contains(err.Error(), sub) {
			t.Errorf("error %v: message does not contain %q", err, sub)
		}
	}
}
