// @intake/core — public API. Frozen in sub-plan 1-v.

// Generated payload types (Phase 0 contract spine)
export * from './generated/payload.js';

// Public client API (frozen in 1-v)
export type { IntakeConfig, ChatMessage, SubmitResult } from './client-types.js';
export { IntakeClient } from './client.js';

// Context capture utilities (exported for widget use)
export { captureClient, capturePageMetadata } from './context.js';
