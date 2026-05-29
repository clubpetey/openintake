import { describe, it, expect, vi, beforeEach } from 'vitest';
import { useIntake } from './useIntake';

// --- Mock @intake/core ---
// We mock the IntakeClient class so no real HTTP calls happen.
const mockInit = vi.fn();
const mockTurn = vi.fn();
const mockSubmit = vi.fn();

vi.mock('@intake/core', async (orig) => {
  const actual = (await orig()) as Record<string, unknown>;
  function IntakeClient() {
    return {
      init: mockInit,
      turn: mockTurn,
      submit: mockSubmit,
    };
  }
  return { ...actual, IntakeClient };
});

describe('useIntake', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('initializes with empty messages and idle state', () => {
    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    expect(intake.messages.value).toEqual([]);
    expect(intake.streaming.value).toBe(false);
    expect(intake.submitting.value).toBe(false);
    expect(intake.result.value).toBeNull();
  });

  it('start() calls client.init() and returns session_id', async () => {
    mockInit.mockResolvedValue({ session_id: 'sess-abc', capabilities: { auth_modes: ['anonymous'], streaming: true } });
    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    const session = await intake.start();
    expect(mockInit).toHaveBeenCalledOnce();
    expect(session.session_id).toBe('sess-abc');
  });

  it('sendTurn() appends user message, streams assistant deltas, sets streaming=true then false', async () => {
    mockInit.mockResolvedValue({ session_id: 'sess-abc', capabilities: { auth_modes: ['anonymous'], streaming: true } });

    // turn() invokes onDelta twice then resolves
    mockTurn.mockImplementation(async (_messages: unknown, onDelta: (d: string) => void) => {
      onDelta('Hello ');
      onDelta('world');
      return { input_tokens: 10, output_tokens: 5 };
    });

    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await intake.start();
    await intake.sendTurn('Hi there');

    expect(intake.messages.value).toHaveLength(2);
    expect(intake.messages.value[0]).toEqual({ role: 'user', content: 'Hi there' });
    expect(intake.messages.value[1]).toEqual({ role: 'assistant', content: 'Hello world' });
    expect(intake.streaming.value).toBe(false); // reset after done
  });

  it('streaming is true during sendTurn and false after', async () => {
    mockInit.mockResolvedValue({ session_id: 'sess-abc', capabilities: { auth_modes: ['anonymous'], streaming: true } });

    let capturedStreaming: boolean | null = null;
    mockTurn.mockImplementation(async (_messages: unknown, onDelta: (d: string) => void) => {
      onDelta('x');
      // By the time onDelta is called, streaming should be true
      return { input_tokens: 1, output_tokens: 1 };
    });

    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await intake.start();

    // Patch onDelta to capture the state at delta time
    mockTurn.mockImplementation(async (_messages: unknown, onDelta: (d: string) => void) => {
      capturedStreaming = intake.streaming.value;
      onDelta('x');
      return { input_tokens: 1, output_tokens: 1 };
    });

    await intake.sendTurn('test');
    expect(capturedStreaming).toBe(true);
    expect(intake.streaming.value).toBe(false);
  });

  it('submit() sets submitting=true, calls client.submit(), stores result, resets submitting', async () => {
    mockInit.mockResolvedValue({ session_id: 'sess-abc', capabilities: { auth_modes: ['anonymous'], streaming: true } });
    mockTurn.mockResolvedValue({ input_tokens: 1, output_tokens: 1 });
    mockSubmit.mockResolvedValue({
      external_id: 'ticket-123',
      external_url: 'http://localhost:9099/intake/ticket-123',
      adapter_name: 'webhook',
      created_at: '2026-05-26T00:00:00Z',
    });

    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await intake.start();
    await intake.sendTurn('My app crashes');
    await intake.submit();

    expect(mockSubmit).toHaveBeenCalledOnce();
    expect(intake.result.value).not.toBeNull();
    expect(intake.result.value?.external_id).toBe('ticket-123');
    expect(intake.submitting.value).toBe(false);
  });

  it('submit() passes all accumulated messages to client.submit()', async () => {
    mockInit.mockResolvedValue({ session_id: 'sess-abc', capabilities: { auth_modes: ['anonymous'], streaming: true } });
    mockTurn.mockResolvedValue({ input_tokens: 1, output_tokens: 1 });
    mockSubmit.mockResolvedValue({
      external_id: 'ticket-xyz',
      external_url: '',
      adapter_name: 'webhook',
      created_at: '2026-05-26T00:00:00Z',
    });

    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await intake.start();
    await intake.sendTurn('First message');
    await intake.submit();

    const submitCall = mockSubmit.mock.calls[0];
    // First arg is the messages array
    expect(submitCall[0]).toContainEqual({ role: 'user', content: 'First message' });
  });

  // --- Error-path tests ---

  it('sendTurn() rejection: sets error, resets streaming, removes empty assistant placeholder', async () => {
    mockInit.mockResolvedValue({ session_id: 'sess-abc', capabilities: { auth_modes: ['anonymous'], streaming: true } });
    mockTurn.mockRejectedValue(new Error('relay down'));

    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await intake.start();
    await intake.sendTurn('hi');

    expect(intake.error.value).toBeTruthy();
    expect(intake.streaming.value).toBe(false);
    // No empty assistant placeholder should remain
    const emptyAssistant = intake.messages.value.find(
      (m) => m.role === 'assistant' && m.content === '',
    );
    expect(emptyAssistant).toBeUndefined();
  });

  it('submit() rejection: sets error and resets submitting', async () => {
    mockInit.mockResolvedValue({ session_id: 'sess-abc', capabilities: { auth_modes: ['anonymous'], streaming: true } });
    mockSubmit.mockRejectedValue(new Error('submit failed'));

    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await intake.start();
    await intake.submit();

    expect(intake.error.value).toBeTruthy();
    expect(intake.submitting.value).toBe(false);
  });

  it('start() rejection: sets error and re-throws', async () => {
    mockInit.mockRejectedValue(new Error('relay down'));

    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await expect(intake.start()).rejects.toThrow('relay down');
    expect(intake.error.value).toBeTruthy();
  });
});

describe('useIntake — Phase 6 attachments surface', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('canAttach is false before start() and when capabilities.attachments is null', async () => {
    mockInit.mockResolvedValue({
      session_id: 'sess-1',
      capabilities: { auth_modes: ['anonymous'], streaming: true },
    });
    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    expect(intake.canAttach.value).toBe(false);
    expect(intake.attachLimits.value).toBeNull();
    await intake.start();
    expect(intake.canAttach.value).toBe(false);
    expect(intake.attachLimits.value).toBeNull();
  });

  it('canAttach is true and attachLimits populated when capabilities.attachments present', async () => {
    mockInit.mockResolvedValue({
      session_id: 'sess-1',
      capabilities: {
        auth_modes: ['anonymous'],
        streaming: true,
        attachments: {
          max_size_bytes: 5_242_880,
          max_total_bytes: 10_485_760,
          allowed_mime_types: ['image/png', 'image/jpeg', 'image/webp'],
        },
      },
    });
    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await intake.start();
    expect(intake.canAttach.value).toBe(true);
    expect(intake.attachLimits.value).toEqual({
      maxSizeBytes: 5_242_880,
      maxTotalBytes: 10_485_760,
      allowedMimeTypes: ['image/png', 'image/jpeg', 'image/webp'],
    });
  });

  it('commitRedacted appends to pendingAttachments with size accounting', async () => {
    mockInit.mockResolvedValue({
      session_id: 'sess-1',
      capabilities: {
        auth_modes: ['anonymous'],
        streaming: true,
        attachments: {
          max_size_bytes: 5_242_880,
          max_total_bytes: 10_485_760,
          allowed_mime_types: ['image/png'],
        },
      },
    });
    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await intake.start();
    // base64 payload of 4 chars decodes to 3 bytes — easy assertion target.
    intake.commitRedacted('data:image/png;base64,AAAA');
    expect(intake.pendingAttachments.value).toHaveLength(1);
    expect(intake.pendingAttachments.value[0].mimeType).toBe('image/png');
    expect(intake.pendingAttachments.value[0].sizeBytes).toBeGreaterThan(0);
  });

  it('commitRedacted on an over-cap dataUrl sets a friendly error and does NOT append', async () => {
    mockInit.mockResolvedValue({
      session_id: 'sess-1',
      capabilities: {
        auth_modes: ['anonymous'],
        streaming: true,
        attachments: {
          max_size_bytes: 4, // 4 bytes — anything non-tiny exceeds it
          max_total_bytes: 1_000,
          allowed_mime_types: ['image/png'],
        },
      },
    });
    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await intake.start();
    // ~600-byte base64 payload — well over 4-byte cap.
    const big = 'data:image/png;base64,' + 'A'.repeat(800);
    intake.commitRedacted(big);
    expect(intake.pendingAttachments.value).toHaveLength(0);
    expect(intake.error.value).toMatch(/too large|smaller region/i);
  });

  it('removeAttachment removes by index', async () => {
    mockInit.mockResolvedValue({
      session_id: 'sess-1',
      capabilities: {
        auth_modes: ['anonymous'],
        streaming: true,
        attachments: {
          max_size_bytes: 5_242_880,
          max_total_bytes: 10_485_760,
          allowed_mime_types: ['image/png'],
        },
      },
    });
    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await intake.start();
    intake.commitRedacted('data:image/png;base64,AAAA');
    intake.commitRedacted('data:image/png;base64,BBBB');
    intake.removeAttachment(0);
    expect(intake.pendingAttachments.value).toHaveLength(1);
    expect(intake.pendingAttachments.value[0].dataUrl).toBe('data:image/png;base64,BBBB');
  });

  it('clearAttachments empties the list', async () => {
    mockInit.mockResolvedValue({
      session_id: 'sess-1',
      capabilities: {
        auth_modes: ['anonymous'],
        streaming: true,
        attachments: {
          max_size_bytes: 5_242_880,
          max_total_bytes: 10_485_760,
          allowed_mime_types: ['image/png'],
        },
      },
    });
    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await intake.start();
    intake.commitRedacted('data:image/png;base64,AAAA');
    intake.clearAttachments();
    expect(intake.pendingAttachments.value).toEqual([]);
  });

  it('submit() threads pendingAttachments into client.submit and clears on success', async () => {
    mockInit.mockResolvedValue({
      session_id: 'sess-1',
      capabilities: {
        auth_modes: ['anonymous'],
        streaming: true,
        attachments: {
          max_size_bytes: 5_242_880,
          max_total_bytes: 10_485_760,
          allowed_mime_types: ['image/png'],
        },
      },
    });
    mockSubmit.mockResolvedValue({
      external_id: 't-1',
      external_url: '',
      adapter_name: 'webhook',
      created_at: '2026-05-28T00:00:00Z',
    });
    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await intake.start();
    intake.commitRedacted('data:image/png;base64,AAAA');
    await intake.submit();
    const [, , attachmentsArg] = mockSubmit.mock.calls[0] as [unknown, unknown, unknown[]];
    expect(attachmentsArg).toHaveLength(1);
    expect((attachmentsArg[0] as { mime_type: string }).mime_type).toBe('image/png');
    expect(intake.pendingAttachments.value).toEqual([]);
  });

  it('submit() does NOT clear pending list on failure', async () => {
    mockInit.mockResolvedValue({
      session_id: 'sess-1',
      capabilities: {
        auth_modes: ['anonymous'],
        streaming: true,
        attachments: {
          max_size_bytes: 5_242_880,
          max_total_bytes: 10_485_760,
          allowed_mime_types: ['image/png'],
        },
      },
    });
    const err = new Error('submit failed: 413 ...') as Error & { code?: string };
    err.code = 'attachment_too_large';
    mockSubmit.mockRejectedValue(err);
    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await intake.start();
    intake.commitRedacted('data:image/png;base64,AAAA');
    await intake.submit();
    expect(intake.pendingAttachments.value).toHaveLength(1);
    expect(intake.error.value).toBe('Screenshot too large — try a smaller region.');
  });

  it('submit() maps each known relay error code to its friendly banner string', async () => {
    mockInit.mockResolvedValue({
      session_id: 'sess-1',
      capabilities: { auth_modes: ['anonymous'], streaming: true },
    });
    const cases: Array<[string, RegExp]> = [
      ['attachment_too_large', /smaller region/i],
      ['attachments_exceed_total', /remove one/i],
      ['attachment_mime_not_allowed', /isn'?t supported/i],
      ['attachment_mime_mismatch', /couldn'?t be verified/i],
      ['attachment_malformed', /couldn'?t be verified/i],
      ['attachment_type_unsupported', /isn'?t supported/i],
      ['attachments_disabled', /disabled on this server/i],
      ['request_body_too_large', /too large to send/i],
    ];
    for (const [code, pattern] of cases) {
      const err = new Error('boom') as Error & { code?: string };
      err.code = code;
      mockSubmit.mockRejectedValueOnce(err);
      const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
      await intake.start();
      await intake.submit();
      expect(intake.error.value).toMatch(pattern);
    }
  });

  it('submit() with an unknown error code falls back to the raw error message', async () => {
    mockInit.mockResolvedValue({
      session_id: 'sess-1',
      capabilities: { auth_modes: ['anonymous'], streaming: true },
    });
    const err = new Error('original message') as Error & { code?: string };
    err.code = 'some_future_unknown_code';
    mockSubmit.mockRejectedValue(err);
    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await intake.start();
    await intake.submit();
    expect(intake.error.value).toBe('original message');
  });

  it('attachAndRedact opens redactorSource and commitRedacted closes it', async () => {
    mockInit.mockResolvedValue({
      session_id: 'sess-1',
      capabilities: {
        auth_modes: ['anonymous'],
        streaming: true,
        attachments: {
          max_size_bytes: 5_242_880,
          max_total_bytes: 10_485_760,
          allowed_mime_types: ['image/png'],
        },
      },
    });
    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await intake.start();
    const fakeCanvas = { width: 10, height: 10 } as unknown as HTMLCanvasElement;
    // Inject a stub capturePage via the composable's option.
    await intake.attachAndRedact(async () => fakeCanvas);
    expect(intake.redactorSource.value).toBe(fakeCanvas);
    intake.commitRedacted('data:image/png;base64,AAAA');
    expect(intake.redactorSource.value).toBeNull();
  });

  it('cancelRedactor() closes the modal without committing', async () => {
    mockInit.mockResolvedValue({
      session_id: 'sess-1',
      capabilities: {
        auth_modes: ['anonymous'],
        streaming: true,
        attachments: {
          max_size_bytes: 5_242_880,
          max_total_bytes: 10_485_760,
          allowed_mime_types: ['image/png'],
        },
      },
    });
    const intake = useIntake({ relayUrl: 'http://localhost:8080', widgetVersion: '0.1.0' });
    await intake.start();
    const fakeCanvas = { width: 10, height: 10 } as unknown as HTMLCanvasElement;
    await intake.attachAndRedact(async () => fakeCanvas);
    intake.cancelRedactor();
    expect(intake.redactorSource.value).toBeNull();
    expect(intake.pendingAttachments.value).toEqual([]);
  });
});
