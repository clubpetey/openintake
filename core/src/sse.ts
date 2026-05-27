import type { SSEFrame } from './types.js';

/**
 * Consumes a ReadableStream<Uint8Array> as an SSE stream.
 * Calls onFrame for each parsed data frame.
 * Resolves when the stream closes.
 *
 * Protocol: each SSE event is separated by a blank line (\n\n).
 * Only lines starting with "data: " are parsed; others are skipped.
 * Comment lines (starting with ":") and blank lines are ignored.
 */

/**
 * Parse and dispatch a single SSE event block (the text between \n\n boundaries).
 * Skips blank blocks, comment lines, and malformed JSON.
 * onFrame is called OUTSIDE the JSON.parse try/catch so callback errors propagate.
 */
function dispatchBlock(block: string, onFrame: (f: SSEFrame) => void): void {
  const trimmed = block.trim();
  if (!trimmed) return;

  for (const line of trimmed.split('\n')) {
    // Ignore SSE comment lines and non-data lines
    if (line.startsWith(':') || !line.startsWith('data: ')) continue;

    const payload = line.slice('data: '.length);
    let frame: SSEFrame;
    try {
      frame = JSON.parse(payload) as SSEFrame;
    } catch {
      // Malformed JSON — skip silently (relay contract guarantees valid JSON)
      continue;
    }
    // onFrame is called OUTSIDE the try/catch so throwing callbacks propagate
    onFrame(frame);
  }
}

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

      // Normalize CRLF line endings from proxies/CDNs before buffering
      const chunk = decoder.decode(value, { stream: true }).replace(/\r\n/g, '\n').replace(/\r/g, '\n');
      buffer += chunk;

      // Split on double newline (SSE event boundary)
      const events = buffer.split('\n\n');
      // Keep the last (possibly incomplete) chunk in the buffer
      buffer = events.pop() ?? '';

      for (const event of events) {
        dispatchBlock(event, onFrame);
      }
    }

    // Flush any remaining buffered content
    const tail = decoder.decode().replace(/\r\n/g, '\n').replace(/\r/g, '\n');
    buffer += tail;
    if (buffer.trim()) {
      dispatchBlock(buffer, onFrame);
    }
  } finally {
    // cancel() is a no-op if the stream is already done; releaseLock always runs
    try {
      await reader.cancel();
    } catch {
      // ignore cancel errors
    }
    reader.releaseLock();
  }
}
