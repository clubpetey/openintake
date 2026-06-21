import { ref, shallowRef, computed, type Ref, type ComputedRef } from 'vue';
import {
  IntakeClient,
  AttachmentList,
  AttachmentTooLargeError,
  AggregateTooLargeError,
  MimeNotAllowedError,
} from '@openintake/core';
import type {
  IntakeConfig,
  ChatMessage,
  SubmitResult,
  PendingAttachment,
  AttachmentLimits,
} from '@openintake/core';
import type { InitResponse } from '@openintake/core';

// Security invariant: the widget NEVER handles provider API keys.
// It only calls the relay through @openintake/core's IntakeClient.
// No code path in this file contacts Anthropic or any LLM provider directly.

export interface UseIntakeOptions {
  relayUrl: string;
  widgetVersion?: string;
  appContext?: Record<string, unknown>;
}

// Phase 6 §8.3 — relay error-code → user-readable banner text mapping.
const ATTACHMENT_ERROR_MESSAGES: Record<string, string> = {
  attachment_too_large: 'Screenshot too large — try a smaller region.',
  attachments_exceed_total: 'Too many attachments — remove one.',
  attachment_mime_not_allowed: "This attachment type isn't supported.",
  attachment_mime_mismatch: "This attachment couldn't be verified — try recapturing.",
  attachment_malformed: "This attachment couldn't be verified — try recapturing.",
  attachment_type_unsupported: "This attachment type isn't supported.",
  attachments_disabled: 'Attachments are disabled on this server.',
  request_body_too_large: 'Your submission is too large to send.',
};

function friendlyErrorMessage(e: unknown): string {
  if (e instanceof Error) {
    const code = (e as Error & { code?: string }).code;
    if (typeof code === 'string' && code in ATTACHMENT_ERROR_MESSAGES) {
      return ATTACHMENT_ERROR_MESSAGES[code];
    }
    return e.message;
  }
  return String(e);
}

// Approximate raw byte size of a data:<mime>;base64,<payload> URL.
function approxBytesFromDataUrl(dataUrl: string): number {
  const comma = dataUrl.indexOf(',');
  if (comma < 0) return 0;
  const payload = dataUrl.slice(comma + 1);
  // base64 encodes 3 bytes per 4 chars; subtract padding.
  const pad = payload.endsWith('==') ? 2 : payload.endsWith('=') ? 1 : 0;
  return Math.floor((payload.length * 3) / 4) - pad;
}

function mimeFromDataUrl(dataUrl: string): string {
  const m = dataUrl.match(/^data:([^;]+);base64,/);
  return m ? m[1] : 'application/octet-stream';
}

export function useIntake(options: UseIntakeOptions) {
  const config: IntakeConfig = {
    relayUrl: options.relayUrl,
    widgetVersion: options.widgetVersion ?? '0.1.0',
    appContext: options.appContext,
  };

  const client = new IntakeClient(config);

  const messages = ref<ChatMessage[]>([]);
  const streaming = ref(false);
  const submitting = ref(false);
  const result = ref<SubmitResult | null>(null);
  const error = ref<string | null>(null);
  const initResponse = ref<InitResponse | null>(null);

  // Phase 6 — attachments state.
  const pendingAttachments = ref<PendingAttachment[]>([]);
  // Use shallowRef so the HTMLCanvasElement is not deep-reactive-proxied —
  // identity must be preserved for downstream `.toBe(canvas)` assertions and
  // for cheap DOM passing.
  const redactorSource: Ref<HTMLCanvasElement | null> = shallowRef(null);
  let attachmentList: AttachmentList | null = null;

  const attachLimits: ComputedRef<AttachmentLimits | null> = computed(() => {
    const caps = initResponse.value?.capabilities.attachments;
    if (!caps) return null;
    return {
      maxSizeBytes: caps.max_size_bytes,
      maxTotalBytes: caps.max_total_bytes,
      allowedMimeTypes: caps.allowed_mime_types,
    };
  });

  const canAttach: ComputedRef<boolean> = computed(() => attachLimits.value !== null);

  function ensureList(): AttachmentList | null {
    if (attachmentList) return attachmentList;
    const limits = attachLimits.value;
    if (!limits) return null;
    attachmentList = new AttachmentList(limits);
    return attachmentList;
  }

  async function start() {
    error.value = null;
    try {
      const res = await client.init();
      initResponse.value = res;
      // Reset the list whenever caps change (fresh session).
      attachmentList = null;
      return res;
    } catch (e) {
      error.value = "Couldn't connect. Please try again.";
      throw e;
    }
  }

  async function sendTurn(text: string) {
    messages.value = [...messages.value, { role: 'user', content: text }];
    const assistantIndex = messages.value.length;
    messages.value = [...messages.value, { role: 'assistant', content: '' }];

    streaming.value = true;
    error.value = null;

    try {
      await client.turn(messages.value.slice(0, assistantIndex), (delta: string) => {
        const updated = [...messages.value];
        updated[assistantIndex] = {
          role: 'assistant',
          content: updated[assistantIndex].content + delta,
        };
        messages.value = updated;
      });
    } catch (e) {
      error.value = friendlyErrorMessage(e);
      const last = messages.value[messages.value.length - 1];
      if (last && last.role === 'assistant' && last.content === '') {
        messages.value = messages.value.slice(0, -1);
      }
    } finally {
      streaming.value = false;
    }
  }

  /**
   * Runs the supplied capture function (default: dynamically imports
   * @openintake/core's capturePage) and opens the redactor modal by setting
   * redactorSource. Tests pass a stub capture; production passes the
   * real capturePage from @openintake/core.
   */
  async function attachAndRedact(capture: () => Promise<HTMLCanvasElement>) {
    if (!canAttach.value) {
      error.value = ATTACHMENT_ERROR_MESSAGES.attachments_disabled;
      return;
    }
    error.value = null;
    try {
      const canvas = await capture();
      redactorSource.value = canvas;
    } catch (e) {
      error.value = friendlyErrorMessage(e);
    }
  }

  /**
   * Closes the redactor modal without committing.
   */
  function cancelRedactor() {
    redactorSource.value = null;
  }

  /**
   * Commits a redacted PNG data URL to the pending list and closes the modal.
   * Translates AttachmentList errors via the same code mapping the relay uses.
   */
  function commitRedacted(dataUrl: string) {
    const list = ensureList();
    if (!list) {
      error.value = ATTACHMENT_ERROR_MESSAGES.attachments_disabled;
      redactorSource.value = null;
      return;
    }
    const sizeBytes = approxBytesFromDataUrl(dataUrl);
    const mimeType = mimeFromDataUrl(dataUrl);
    const att: PendingAttachment = {
      type: 'screenshot',
      mimeType,
      dataUrl,
      sizeBytes,
      label: `screenshot ${pendingAttachments.value.length + 1}`,
    };
    try {
      list.add(att);
      pendingAttachments.value = [...list.items()];
      error.value = null;
    } catch (e) {
      if (
        e instanceof AttachmentTooLargeError ||
        e instanceof AggregateTooLargeError ||
        e instanceof MimeNotAllowedError
      ) {
        error.value = ATTACHMENT_ERROR_MESSAGES[e.code] ?? e.message;
      } else {
        error.value = friendlyErrorMessage(e);
      }
    } finally {
      redactorSource.value = null;
    }
  }

  function removeAttachment(index: number) {
    const list = ensureList();
    if (!list) return;
    try {
      list.remove(index);
      pendingAttachments.value = [...list.items()];
    } catch (e) {
      error.value = friendlyErrorMessage(e);
    }
  }

  function clearAttachments() {
    const list = ensureList();
    if (!list) {
      pendingAttachments.value = [];
      return;
    }
    list.clear();
    pendingAttachments.value = [];
  }

  async function submit(routingHint?: string) {
    submitting.value = true;
    error.value = null;
    try {
      const wireAttachments = pendingAttachments.value.map((a) => ({
        type: 'screenshot' as const,
        mime_type: a.mimeType,
        url: a.dataUrl,
        ...(a.label !== undefined ? { label: a.label } : {}),
      }));
      result.value = await client.submit(
        messages.value,
        routingHint,
        wireAttachments.length > 0 ? wireAttachments : undefined,
      );
      // Clear on success only.
      const list = ensureList();
      if (list) list.clear();
      pendingAttachments.value = [];
    } catch (e) {
      error.value = friendlyErrorMessage(e);
    } finally {
      submitting.value = false;
    }
  }

  return {
    messages,
    streaming,
    submitting,
    result,
    error,
    initResponse,
    pendingAttachments,
    redactorSource,
    canAttach,
    attachLimits,
    start,
    sendTurn,
    submit,
    attachAndRedact,
    cancelRedactor,
    commitRedacted,
    removeAttachment,
    clearAttachments,
  };
}
