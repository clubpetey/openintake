// Phase 6 (6-i): optional Capabilities() seam for adapters that advertise
// what attachment MIME types they accept. The frozen Adapter interface is
// UNCHANGED — Capabilities is a separate, optional interface discovered via
// a type assertion (see server.computeAttachmentsCaps).
//
// In v0 all five built-in adapters return the same list
// ["image/png","image/jpeg","image/webp"]. The struct exists so v1+ can
// specialise per-adapter (e.g. a chat-only adapter that accepts only PNG)
// without touching every call site.
package adapter

// Capabilities reports what an adapter supports. v0 carries a single field;
// future versions may add MaxBytes, attachment-count caps, etc.
type Capabilities struct {
	AcceptedMIMETypes []string // empty = no attachments supported
}

// CapableAdapter is the OPTIONAL interface adapters implement to advertise
// attachment capabilities. Adapters that don't implement it advertise no
// capabilities (effectively []string{}). The frozen Adapter interface in
// adapter.go is UNCHANGED — Phase 6 callers use a type assertion to discover
// the optional method.
type CapableAdapter interface {
	Capabilities() Capabilities
}
