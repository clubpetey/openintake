import type { SSEFrame } from './types.js';

/**
 * Consumes a ReadableStream<Uint8Array> as an SSE stream.
 * Calls onFrame for each parsed data frame.
 * Resolves when the stream closes.
 *
 * Protocol: each SSE event is separated by a blank line (\n\n).
 * Only lines starting with "data: " are parsed; others are skipped.
 */
export async function consumeSSE(
  stream: ReadableStream<Uint8Array>,
  onFrame: (frame: SSEFrame) => void
): Promise<void> {
  const decoder = new TextDecoder();
  const reader = stream.getReader();
  let buffer = '';

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });

      // Split on double newline (SSE event boundary)
      const events = buffer.split('\n\n');
      // Keep the last (possibly incomplete) chunk in the buffer
      buffer = events.pop() ?? '';

      for (const event of events) {
        for (const line of event.split('\n')) {
          if (!line.startsWith('data: ')) continue;
          const payload = line.slice('data: '.length);
          try {
            const frame = JSON.parse(payload) as SSEFrame;
            onFrame(frame);
          } catch {
            // Malformed JSON — skip silently (relay contract guarantees valid JSON)
          }
        }
      }
    }

    // Flush any remaining buffered content
    buffer += decoder.decode();
    if (buffer.trim()) {
      for (const line of buffer.split('\n')) {
        if (!line.startsWith('data: ')) continue;
        const payload = line.slice('data: '.length);
        try {
          const frame = JSON.parse(payload) as SSEFrame;
          onFrame(frame);
        } catch {
          // skip
        }
      }
    }
  } finally {
    reader.releaseLock();
  }
}
