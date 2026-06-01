import { describe, it, expect } from 'vitest';
import { consumeSSE } from './sse.js';
import type { SSEFrame } from './types.js';

/** Helper: turn a string into a ReadableStream<Uint8Array> */
function streamFrom(text: string): ReadableStream<Uint8Array> {
  const enc = new TextEncoder();
  return new ReadableStream({
    start(controller) {
      controller.enqueue(enc.encode(text));
      controller.close();
    },
  });
}

describe('consumeSSE', () => {
  it('calls onFrame for a single delta event', async () => {
    const frames: SSEFrame[] = [];
    const stream = streamFrom('data: {"delta":"hello"}\n\n');
    await consumeSSE(stream, (f) => frames.push(f));
    expect(frames).toHaveLength(1);
    expect(frames[0]).toEqual({ delta: 'hello' });
  });

  it('calls onFrame for done event', async () => {
    const frames: SSEFrame[] = [];
    const stream = streamFrom('data: {"done":true,"input_tokens":5,"output_tokens":10}\n\n');
    await consumeSSE(stream, (f) => frames.push(f));
    expect(frames).toHaveLength(1);
    expect(frames[0]).toEqual({ done: true, input_tokens: 5, output_tokens: 10 });
  });

  it('calls onFrame for error event', async () => {
    const frames: SSEFrame[] = [];
    const stream = streamFrom('data: {"error":"upstream failed"}\n\n');
    await consumeSSE(stream, (f) => frames.push(f));
    expect(frames).toHaveLength(1);
    expect(frames[0]).toEqual({ error: 'upstream failed' });
  });

  it('parses multiple events in sequence', async () => {
    const frames: SSEFrame[] = [];
    const raw =
      'data: {"delta":"a"}\n\n' +
      'data: {"delta":"b"}\n\n' +
      'data: {"done":true,"input_tokens":1,"output_tokens":2}\n\n';
    await consumeSSE(streamFrom(raw), (f) => frames.push(f));
    expect(frames).toHaveLength(3);
    expect(frames[0]).toEqual({ delta: 'a' });
    expect(frames[1]).toEqual({ delta: 'b' });
    expect(frames[2]).toEqual({ done: true, input_tokens: 1, output_tokens: 2 });
  });

  it('ignores lines that do not start with "data: "', async () => {
    const frames: SSEFrame[] = [];
    // SSE spec allows comment lines ": comment" and event: lines
    const raw = ': keep-alive\n\ndata: {"delta":"x"}\n\n';
    await consumeSSE(streamFrom(raw), (f) => frames.push(f));
    expect(frames).toHaveLength(1);
    expect(frames[0]).toEqual({ delta: 'x' });
  });

  it('handles a stream split across multiple chunks', async () => {
    const enc = new TextEncoder();
    const parts = ['data: {"del', 'ta":"split"}\n\n'];
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        for (const p of parts) controller.enqueue(enc.encode(p));
        controller.close();
      },
    });
    const frames: SSEFrame[] = [];
    await consumeSSE(stream, (f) => frames.push(f));
    expect(frames).toHaveLength(1);
    expect(frames[0]).toEqual({ delta: 'split' });
  });
});
