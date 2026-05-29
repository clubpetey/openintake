// @intake/core — public API. Frozen in sub-plan 1-v.

// Generated payload types (Phase 0 contract spine)
export * from './generated/payload.js';

// Public client API (frozen in 1-v)
export type { IntakeConfig, ChatMessage, SubmitResult } from './client-types.js';
export { IntakeClient } from './client.js';

// Context capture utilities (exported for widget use)
export { captureClient, capturePageMetadata } from './context.js';

// Phase 6 — attachments + capture
export {
  AttachmentList,
  AttachmentTooLargeError,
  AggregateTooLargeError,
  MimeNotAllowedError,
} from './attachments.js';
export type { PendingAttachment, AttachmentLimits } from './attachments.js';
export { setHtml2Canvas, capturePage, canvasToDataURL } from './capture.js';
export type { Html2CanvasFn } from './capture.js';
