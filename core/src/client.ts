import type { IntakeConfig, ChatMessage, SubmitResult } from './client-types.js';
import type { InitResponse, SubmitRequest, SubmitResponse, SSEFrame } from './types.js';
import { consumeSSE } from './sse.js';
import { captureClient, capturePageMetadata } from './context.js';

export class IntakeClient {
  private readonly config: IntakeConfig;
  private readonly fetch: typeof globalThis.fetch;
  private sessionId: string | null = null;
  private bearerToken: string | null = null;

  constructor(config: IntakeConfig, fetchImpl?: typeof globalThis.fetch) {
    this.config = config;
    this.fetch = fetchImpl ?? globalThis.fetch;
  }

  /** Set (or clear) the bearer token sent in Authorization headers for turn() and submit(). */
  setBearerToken(token: string | null): void {
    this.bearerToken = token;
  }

  async init(): Promise<InitResponse> {
    const url = `${this.config.relayUrl}/v1/intake/init`;
    const res = await this.fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    });

    if (!res.ok) {
      throw new Error(`init failed: ${res.status}`);
    }

    const data = (await res.json()) as InitResponse;
    this.sessionId = data.session_id;
    return data;
  }

  async turn(
    messages: ChatMessage[],
    onDelta: (delta: string) => void
  ): Promise<{ input_tokens: number; output_tokens: number }> {
    if (this.sessionId === null) {
      throw new Error('IntakeClient: call init() before turn()');
    }

    const url = `${this.config.relayUrl}/v1/intake/turn`;
    const turnHeaders: Record<string, string> = {
      'Content-Type': 'application/json',
      'X-Intake-Session': this.sessionId,
    };
    if (this.bearerToken !== null) {
      turnHeaders['Authorization'] = `Bearer ${this.bearerToken}`;
    }
    const res = await this.fetch(url, {
      method: 'POST',
      headers: turnHeaders,
      body: JSON.stringify({ messages }),
    });

    if (!res.ok) {
      throw new Error(`turn failed: ${res.status}`);
    }

    if (!res.body) {
      throw new Error('turn: response has no body');
    }

    return new Promise<{ input_tokens: number; output_tokens: number }>(
      (resolve, reject) => {
        let settled = false;
        const settle = (fn: () => void) => {
          if (!settled) {
            settled = true;
            fn();
          }
        };

        const onFrame = (frame: SSEFrame) => {
          if ('error' in frame) {
            settle(() => reject(new Error(frame.error)));
          } else if ('done' in frame && frame.done) {
            settle(() =>
              resolve({
                input_tokens: frame.input_tokens,
                output_tokens: frame.output_tokens,
              })
            );
          } else if ('delta' in frame) {
            onDelta(frame.delta);
          }
        };

        consumeSSE(res.body as ReadableStream<Uint8Array>, onFrame).then(
          () => settle(() => reject(new Error('turn: stream ended without a done frame'))),
          (err: unknown) => settle(() => reject(err instanceof Error ? err : new Error(String(err)))),
        );
      }
    );
  }

  async submit(
    messages: ChatMessage[],
    routingHint?: string
  ): Promise<SubmitResult> {
    if (this.sessionId === null) {
      throw new Error('IntakeClient: call init() before submit()');
    }

    const clientInfo = captureClient(this.config.widgetVersion);
    const pageMetadata = capturePageMetadata();

    const body: SubmitRequest = {
      messages,
      client: clientInfo,
      user_claims: {},
      context: {
        app_context: this.config.appContext ?? {},
        page_metadata: pageMetadata,
      },
      routing_hint: routingHint ?? null,
    };

    const url = `${this.config.relayUrl}/v1/intake/submit`;
    const submitHeaders: Record<string, string> = {
      'Content-Type': 'application/json',
      'X-Intake-Session': this.sessionId,
    };
    if (this.bearerToken !== null) {
      submitHeaders['Authorization'] = `Bearer ${this.bearerToken}`;
    }
    const res = await this.fetch(url, {
      method: 'POST',
      headers: submitHeaders,
      body: JSON.stringify(body),
    });

    if (!res.ok) {
      const body = await res.text().catch(() => '');
      throw new Error(`submit failed: ${res.status} ${body}`);
    }

    return (await res.json()) as SubmitResponse;
  }
}
