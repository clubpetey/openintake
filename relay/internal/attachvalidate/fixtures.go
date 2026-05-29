package attachvalidate

// Minimal-header byte fixtures for the three v0 MIME types. These are the
// smallest byte sequences that net/http.DetectContentType recognises as the
// claimed format — sufficient for the magic-byte path. Hexdumps below.
//
// They are exported via the GoldenXxx helpers so the test file can build
// data: URLs from them without colocating testdata files (CI keeps the
// package self-contained).

// pngMagic is the 8-byte PNG signature followed by a minimal IHDR-less stub.
// net/http.DetectContentType requires the 8 magic bytes and an IHDR-like
// pattern (\x89PNG\r\n\x1a\n) to return "image/png".
var pngMagic = []byte{
	0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A,
	// padding so DetectContentType has at least the 8 magic bytes; the rest
	// is irrelevant to detection.
	0x00, 0x00, 0x00, 0x0D, 'I', 'H', 'D', 'R',
}

// jpegMagic is the SOI marker (\xFF\xD8\xFF) plus enough header to satisfy
// stdlib sniffing.
var jpegMagic = []byte{
	0xFF, 0xD8, 0xFF, 0xE0,
	0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01,
	0x01, 0x00, 0x00, 0x01,
}

// webpMagic is the "RIFF...WEBP" container header.
var webpMagic = []byte{
	'R', 'I', 'F', 'F',
	0x00, 0x00, 0x00, 0x00, // file length (don't care for sniff)
	'W', 'E', 'B', 'P',
	'V', 'P', '8', ' ',
}

// GoldenPNG returns the minimal valid PNG header (sniffable by
// net/http.DetectContentType as image/png). Exported for tests.
func GoldenPNG() []byte { return append([]byte(nil), pngMagic...) }

// GoldenJPEG returns the minimal valid JPEG header.
func GoldenJPEG() []byte { return append([]byte(nil), jpegMagic...) }

// GoldenWebP returns the minimal valid WebP header.
func GoldenWebP() []byte { return append([]byte(nil), webpMagic...) }
