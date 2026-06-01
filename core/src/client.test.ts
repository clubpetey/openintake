import { describe, it, expect, vi } from 'vitest';
import { IntakeClient } from './client.js';
import type { IntakeConfig, ChatMessage } from './client-types.js';

// A fetch stub that returns a specific JSON response
function makeFetch(
  status: number,
  body: unknown,
  headers: Record<string, string> = { 'content-type': 'application/json' },
): typeof fetch {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    headers: {
      get: (k: string) => headers[k.toLowerCase()] ?? null,
    },
    json: () => Promise.resolve(body),
    body: null,
  } as unknown as Response);
}

const BASE_CONFIG: IntakeConfig = {
  relayUrl: 'http://localhost:8080',
  widgetVersion: '0.1.0',
};

describe('IntakeClient.init()', () => {
  it('POSTs to /v1/intake/init and returns session_id + capabilities', async () => {
    const mockFetch = makeFetch(200, {
      session_id: 'sess-abc',
      capabilities: { auth_modes: ['anonymous'], streaming: true },
    });

    const client = new IntakeClient(BASE_CONFIG, mockFetch);
    const result = await client.init();

    expect(mockFetch).toHaveBeenCalledOnce();
    const [url, opts] = (mockFetch as ReturnType<typeof vi.fn>).mock.calls[0] as [
      string,
      RequestInit,
    ];
    expect(url).toBe('http://localhost:8080/v1/intake/init');
    expect(opts.method).toBe('POST');

    expect(result.session_id).toBe('sess-abc');
    expect(result.capabilities.auth_modes).toContain('anonymous');
    expect(result.capabilities.streaming).toBe(true);
  });

  it('throws on non-2xx response from /init', async () => {
    const mockFetch = makeFetch(500, { error: { code: 'internal', message: 'boom' } });
    const client = new IntakeClient(BASE_CONFIG, mockFetch);
    await expect(client.init()).rejects.toThrow(/500/);
  });

  it('stores session_id and returns it from init()', async () => {
    const mockFetch = makeFetch(200, {
      session_id: 'stored-sess',
      capabilities: { auth_modes: ['anonymous'], streaming: true },
    });
    const client = new IntakeClient(BASE_CONFIG, mockFetch);
    const result = await client.init();
    // init() must return the session_id so callers can observe it
    expect(result.session_id).toBe('stored-sess');
    // A subsequent turn() sends that session_id in the X-Intake-Session header
    // (covered by the turn() 'sends X-Intake-Session header' test)
  });
});

// Helper to build a ReadableStream from an SSE string (for turn() tests)
function sseStream(raw: string): ReadableStream<Uint8Array> {
  const enc = new TextEncoder();
  return new ReadableStream({
    start(controller) {
      controller.enqueue(enc.encode(raw));
      controller.close();
    },
  });
}

describe('IntakeClient.turn()', () => {
  it('sends X-Intake-Session header on turn()', async () => {
    const calls: Array<[string, RequestInit]> = [];
    const spyFetch = vi.fn((...args: Parameters<typeof fetch>) => {
      calls.push(args as [string, RequestInit]);
      const [url] = args as [string, RequestInit];
      if ((url as string).endsWith('/init')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          headers: { get: () => 'application/json' },
          json: () =>
            Promise.resolve({
              session_id: 'hdr-sess',
              capabilities: { auth_modes: ['anonymous'], streaming: true },
            }),
          body: null,
        } as unknown as Response);
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => 'text/event-stream' },
        body: sseStream('data: {"done":true,"input_tokens":1,"output_tokens":2}\n\n'),
        json: () => Promise.reject(new Error('streaming')),
      } as unknown as Response);
    });

    const client = new IntakeClient(BASE_CONFIG, spyFetch);
    await client.init();
    await client.turn([{ role: 'user', content: 'hello' }], () => {});

    expect(calls).toHaveLength(2);
    const [, turnOpts] = calls[1];
    expect((turnOpts.headers as Record<string, string>)['X-Intake-Session']).toBe('hdr-sess');
  });

  it('calls onDelta for each delta frame and resolves with token counts', async () => {
    const sseBody =
      'data: {"delta":"foo"}\n\n' +
      'data: {"delta":"bar"}\n\n' +
      'data: {"done":true,"input_tokens":3,"output_tokens":7}\n\n';

    const spyFetch = vi.fn((...args: Parameters<typeof fetch>) => {
      const [url] = args as [string, RequestInit];
      if ((url as string).endsWith('/init')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          headers: { get: () => 'application/json' },
          json: () =>
            Promise.resolve({
              session_id: 'delta-sess',
              capabilities: { auth_modes: ['anonymous'], streaming: true },
            }),
          body: null,
        } as unknown as Response);
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => 'text/event-stream' },
        body: sseStream(sseBody),
        json: () => Promise.reject(new Error('streaming')),
      } as unknown as Response);
    });

    const client = new IntakeClient(BASE_CONFIG, spyFetch);
    await client.init();

    const deltas: string[] = [];
    const result = await client.turn([{ role: 'user', content: 'test' }], (d) => deltas.push(d));

    expect(deltas).toEqual(['foo', 'bar']);
    expect(result).toEqual({ input_tokens: 3, output_tokens: 7 });
  });

  it('rejects when an SSE error frame is received', async () => {
    const spyFetch = vi.fn((...args: Parameters<typeof fetch>) => {
      const [url] = args as [string, RequestInit];
      if ((url as string).endsWith('/init')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          headers: { get: () => 'application/json' },
          json: () =>
            Promise.resolve({
              session_id: 'err-sess',
              capabilities: { auth_modes: ['anonymous'], streaming: true },
            }),
          body: null,
        } as unknown as Response);
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => 'text/event-stream' },
        body: sseStream('data: {"error":"upstream provider failed"}\n\n'),
        json: () => Promise.reject(new Error('streaming')),
      } as unknown as Response);
    });

    const client = new IntakeClient(BASE_CONFIG, spyFetch);
    await client.init();

    await expect(client.turn([{ role: 'user', content: 'test' }], () => {})).rejects.toThrow(
      'upstream provider failed',
    );
  });

  it('throws if turn() is called before init()', async () => {
    const client = new IntakeClient(BASE_CONFIG, vi.fn());
    await expect(client.turn([{ role: 'user', content: 'hi' }], () => {})).rejects.toThrow(
      'init()',
    );
  });

  it('rejects when the relay responds non-2xx (503) before streaming', async () => {
    const spyFetch = vi.fn((...args: Parameters<typeof fetch>) => {
      const [url] = args as [string, RequestInit];
      if ((url as string).endsWith('/init')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          headers: { get: () => 'application/json' },
          json: () =>
            Promise.resolve({
              session_id: 'fail-sess',
              capabilities: { auth_modes: ['anonymous'], streaming: true },
            }),
          body: null,
        } as unknown as Response);
      }
      return Promise.resolve({
        ok: false,
        status: 503,
        headers: { get: () => 'application/json' },
        json: () => Promise.resolve({ error: { code: 'unavailable', message: 'service down' } }),
        body: null,
      } as unknown as Response);
    });

    const client = new IntakeClient(BASE_CONFIG, spyFetch);
    await client.init();
    await expect(client.turn([{ role: 'user', content: 'hello' }], () => {})).rejects.toThrow(
      /503/,
    );
  });

  it('rejects (does not hang) when stream closes without a done frame', async () => {
    // Stream contains only delta frames — no done or error frame — then closes cleanly.
    // turn() must reject with a protocol error rather than hanging forever.
    const sseBody = 'data: {"delta":"foo"}\n\n' + 'data: {"delta":"bar"}\n\n';

    const spyFetch = vi.fn((...args: Parameters<typeof fetch>) => {
      const [url] = args as [string, RequestInit];
      if ((url as string).endsWith('/init')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          headers: { get: () => 'application/json' },
          json: () =>
            Promise.resolve({
              session_id: 'nodone-sess',
              capabilities: { auth_modes: ['anonymous'], streaming: true },
            }),
          body: null,
        } as unknown as Response);
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => 'text/event-stream' },
        body: sseStream(sseBody),
        json: () => Promise.reject(new Error('streaming')),
      } as unknown as Response);
    });

    const client = new IntakeClient(BASE_CONFIG, spyFetch);
    await client.init();

    // The test completing proves it does not hang; the error message is the proof of correct rejection.
    await expect(client.turn([{ role: 'user', content: 'hello' }], () => {})).rejects.toThrow(
      /stream ended without a done frame/,
    );
  });
});

describe('IntakeClient.submit()', () => {
  it('POSTs SubmitRequest to /v1/intake/submit with X-Intake-Session header', async () => {
    const submitResponse = {
      external_id: 'ticket-123',
      external_url: 'https://example.com/tickets/123',
      adapter_name: 'webhook',
      created_at: '2026-05-26T00:00:00Z',
    };

    const calls: Array<[string, RequestInit]> = [];
    const spyFetch = vi.fn((...args: Parameters<typeof fetch>) => {
      calls.push(args as [string, RequestInit]);
      const [url] = args as [string, RequestInit];
      if ((url as string).endsWith('/init')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          headers: { get: () => 'application/json' },
          json: () =>
            Promise.resolve({
              session_id: 'sub-sess',
              capabilities: { auth_modes: ['anonymous'], streaming: true },
            }),
          body: null,
        } as unknown as Response);
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => 'application/json' },
        json: () => Promise.resolve(submitResponse),
        body: null,
      } as unknown as Response);
    });

    const client = new IntakeClient({ ...BASE_CONFIG, appContext: { tenant: 'acme' } }, spyFetch);
    await client.init();

    const messages: ChatMessage[] = [
      { role: 'user', content: 'I found a bug' },
      { role: 'assistant', content: 'Tell me more.' },
    ];
    const result = await client.submit(messages, 'webhook');

    expect(calls).toHaveLength(2);
    const [submitUrl, submitOpts] = calls[1];
    expect(submitUrl).toBe('http://localhost:8080/v1/intake/submit');
    expect(submitOpts.method).toBe('POST');
    expect((submitOpts.headers as Record<string, string>)['X-Intake-Session']).toBe('sub-sess');

    const body = JSON.parse(submitOpts.body as string) as Record<string, unknown>;
    expect((body['messages'] as unknown[]).length).toBe(2);
    expect((body['context'] as Record<string, unknown>)['app_context']).toEqual({ tenant: 'acme' });
    expect(body['routing_hint']).toBe('webhook');

    // client fields present
    const clientField = body['client'] as Record<string, unknown>;
    expect(clientField['widget_version']).toBe('0.1.0');
    // url/referrer/user_agent/viewport/locale come from captureClient (SSR = empty in Node)
    expect(typeof clientField['url']).toBe('string');

    expect(result).toEqual(submitResponse);
  });

  it('sets routing_hint to null when not provided', async () => {
    const calls: Array<[string, RequestInit]> = [];
    const spyFetch = vi.fn((...args: Parameters<typeof fetch>) => {
      calls.push(args as [string, RequestInit]);
      const [url] = args as [string, RequestInit];
      if ((url as string).endsWith('/init')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          headers: { get: () => 'application/json' },
          json: () =>
            Promise.resolve({
              session_id: 'sub-sess2',
              capabilities: { auth_modes: ['anonymous'], streaming: true },
            }),
          body: null,
        } as unknown as Response);
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => 'application/json' },
        json: () =>
          Promise.resolve({
            external_id: 'x',
            external_url: '',
            adapter_name: 'webhook',
            created_at: '',
          }),
        body: null,
      } as unknown as Response);
    });

    const client = new IntakeClient(BASE_CONFIG, spyFetch);
    await client.init();
    await client.submit([{ role: 'user', content: 'test' }]);

    const [, submitOpts] = calls[1];
    const body = JSON.parse(submitOpts.body as string) as Record<string, unknown>;
    expect(body['routing_hint']).toBeNull();
  });

  it('throws on non-2xx response from /submit', async () => {
    const spyFetch = vi.fn((...args: Parameters<typeof fetch>) => {
      const [url] = args as [string, RequestInit];
      if ((url as string).endsWith('/init')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          headers: { get: () => 'application/json' },
          json: () =>
            Promise.resolve({
              session_id: 'sub-err',
              capabilities: { auth_modes: ['anonymous'], streaming: true },
            }),
          body: null,
        } as unknown as Response);
      }
      return Promise.resolve({
        ok: false,
        status: 502,
        headers: { get: () => 'application/json' },
        json: () => Promise.resolve({ error: { code: 'adapter_error', message: 'webhook down' } }),
        body: null,
      } as unknown as Response);
    });

    const client = new IntakeClient(BASE_CONFIG, spyFetch);
    await client.init();
    await expect(client.submit([{ role: 'user', content: 'test' }])).rejects.toThrow(/502/);
  });

  it('throws if submit() is called before init()', async () => {
    const client = new IntakeClient(BASE_CONFIG, vi.fn());
    await expect(client.submit([{ role: 'user', content: 'hi' }])).rejects.toThrow('init()');
  });
});

describe('IntakeClient.submit() — attachments threading (Phase 6)', () => {
  it('omits attachments[] from the POST body when not provided', async () => {
    const calls: Array<[string, RequestInit]> = [];
    const fetchStub = vi.fn().mockImplementation((url: string, opts: RequestInit) => {
      calls.push([url, opts]);
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => 'application/json' },
        json: () =>
          Promise.resolve({
            external_id: 'id-1',
            external_url: '',
            adapter_name: 'webhook',
            created_at: '2026-05-28T00:00:00Z',
          }),
      } as unknown as Response);
    });
    const client = new IntakeClient(BASE_CONFIG, fetchStub as unknown as typeof fetch);
    // Need a session_id before submit() is allowed; reuse init() flow.
    fetchStub.mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: { get: () => 'application/json' },
      json: () =>
        Promise.resolve({
          session_id: 'sess-1',
          capabilities: { auth_modes: ['anonymous'], streaming: true },
        }),
    } as unknown as Response);
    await client.init();

    await client.submit([{ role: 'user', content: 'hi' }]);

    const submitCall = calls[calls.length - 1];
    const body = JSON.parse(submitCall[1].body as string) as Record<string, unknown>;
    expect('attachments' in body).toBe(false);
  });

  it('includes attachments[] in the POST body when non-empty', async () => {
    const calls: Array<[string, RequestInit]> = [];
    const fetchStub = vi.fn().mockImplementation((url: string, opts: RequestInit) => {
      calls.push([url, opts]);
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => 'application/json' },
        json: () =>
          Promise.resolve({
            external_id: 'id-1',
            external_url: '',
            adapter_name: 'webhook',
            created_at: '2026-05-28T00:00:00Z',
          }),
      } as unknown as Response);
    });
    const client = new IntakeClient(BASE_CONFIG, fetchStub as unknown as typeof fetch);
    fetchStub.mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: { get: () => 'application/json' },
      json: () =>
        Promise.resolve({
          session_id: 'sess-1',
          capabilities: { auth_modes: ['anonymous'], streaming: true },
        }),
    } as unknown as Response);
    await client.init();

    await client.submit([{ role: 'user', content: 'hi' }], undefined, [
      {
        type: 'screenshot',
        mime_type: 'image/png',
        url: 'data:image/png;base64,AAAA',
        label: 'screenshot 1',
      },
    ]);

    const submitCall = calls[calls.length - 1];
    const body = JSON.parse(submitCall[1].body as string) as { attachments: unknown[] };
    expect(body.attachments).toHaveLength(1);
    expect(body.attachments[0]).toMatchObject({
      type: 'screenshot',
      mime_type: 'image/png',
      url: 'data:image/png;base64,AAAA',
      label: 'screenshot 1',
    });
  });

  it('omits attachments[] when an empty array is passed', async () => {
    const calls: Array<[string, RequestInit]> = [];
    const fetchStub = vi.fn().mockImplementation((url: string, opts: RequestInit) => {
      calls.push([url, opts]);
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: { get: () => 'application/json' },
        json: () =>
          Promise.resolve({
            external_id: 'id-1',
            external_url: '',
            adapter_name: 'webhook',
            created_at: '2026-05-28T00:00:00Z',
          }),
      } as unknown as Response);
    });
    const client = new IntakeClient(BASE_CONFIG, fetchStub as unknown as typeof fetch);
    fetchStub.mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: { get: () => 'application/json' },
      json: () =>
        Promise.resolve({
          session_id: 'sess-1',
          capabilities: { auth_modes: ['anonymous'], streaming: true },
        }),
    } as unknown as Response);
    await client.init();

    await client.submit([{ role: 'user', content: 'hi' }], undefined, []);

    const body = JSON.parse(calls[calls.length - 1][1].body as string) as Record<string, unknown>;
    expect('attachments' in body).toBe(false);
  });

  it('non-2xx with attachment ErrorEnvelope: thrown Error carries `code` property', async () => {
    const fetchStub = vi.fn();
    // First call: init OK
    fetchStub.mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: { get: () => 'application/json' },
      json: () =>
        Promise.resolve({
          session_id: 'sess-1',
          capabilities: { auth_modes: ['anonymous'], streaming: true },
        }),
    } as unknown as Response);
    // Second call: submit returns 413 with an ErrorEnvelope
    fetchStub.mockResolvedValueOnce({
      ok: false,
      status: 413,
      headers: { get: () => 'application/json' },
      text: () =>
        Promise.resolve(
          JSON.stringify({
            error: { code: 'attachment_too_large', message: 'attachment exceeds max_size_bytes' },
          }),
        ),
    } as unknown as Response);

    const client = new IntakeClient(BASE_CONFIG, fetchStub as unknown as typeof fetch);
    await client.init();
    try {
      await client.submit([{ role: 'user', content: 'hi' }]);
      throw new Error('expected submit to throw');
    } catch (e) {
      expect(e).toBeInstanceOf(Error);
      expect((e as Error & { code?: string }).code).toBe('attachment_too_large');
    }
  });

  it('non-2xx with non-JSON body: thrown Error has no `code`, retains status in message', async () => {
    const fetchStub = vi.fn();
    fetchStub.mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: { get: () => 'application/json' },
      json: () =>
        Promise.resolve({
          session_id: 'sess-1',
          capabilities: { auth_modes: ['anonymous'], streaming: true },
        }),
    } as unknown as Response);
    fetchStub.mockResolvedValueOnce({
      ok: false,
      status: 500,
      headers: { get: () => 'text/plain' },
      text: () => Promise.resolve('internal'),
    } as unknown as Response);

    const client = new IntakeClient(BASE_CONFIG, fetchStub as unknown as typeof fetch);
    await client.init();
    try {
      await client.submit([{ role: 'user', content: 'hi' }]);
      throw new Error('expected submit to throw');
    } catch (e) {
      expect((e as Error & { code?: string }).code).toBeUndefined();
      expect((e as Error).message).toMatch(/500/);
    }
  });
});

describe('IntakeClient.init() — capabilities.attachments parse (Phase 6)', () => {
  it('parses capabilities.attachments when present', async () => {
    const mockFetch = makeFetch(200, {
      session_id: 'sess-1',
      capabilities: {
        auth_modes: ['anonymous'],
        streaming: true,
        attachments: {
          max_size_bytes: 5242880,
          max_total_bytes: 10485760,
          allowed_mime_types: ['image/png', 'image/jpeg', 'image/webp'],
        },
      },
    });
    const client = new IntakeClient(BASE_CONFIG, mockFetch);
    const result = await client.init();
    expect(result.capabilities.attachments).toBeDefined();
    expect(result.capabilities.attachments?.max_size_bytes).toBe(5242880);
    expect(result.capabilities.attachments?.allowed_mime_types).toEqual([
      'image/png',
      'image/jpeg',
      'image/webp',
    ]);
  });

  it('leaves capabilities.attachments undefined when relay omits the block', async () => {
    const mockFetch = makeFetch(200, {
      session_id: 'sess-1',
      capabilities: { auth_modes: ['anonymous'], streaming: true },
    });
    const client = new IntakeClient(BASE_CONFIG, mockFetch);
    const result = await client.init();
    expect(result.capabilities.attachments).toBeUndefined();
  });
});
