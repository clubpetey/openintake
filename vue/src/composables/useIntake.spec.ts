import { describe, it, expect, vi, beforeEach } from 'vitest';
import { useIntake } from './useIntake';

// --- Mock @intake/core ---
// We mock the IntakeClient class so no real HTTP calls happen.
const mockInit = vi.fn();
const mockTurn = vi.fn();
const mockSubmit = vi.fn();

vi.mock('@intake/core', () => {
  function IntakeClient() {
    return {
      init: mockInit,
      turn: mockTurn,
      submit: mockSubmit,
    };
  }
  return { IntakeClient };
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
});
