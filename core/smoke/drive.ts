/**
 * Smoke script for @intake/core.
 * Drives init → turn → submit against a running relay.
 *
 * Usage:
 *   RELAY_URL=http://localhost:8080 npx tsx core/smoke/drive.ts
 *
 * Prerequisites:
 *   - relay running (sub-plans 1-i..1-iv complete)
 *   - ANTHROPIC_API_KEY exported in the relay's environment
 *   - adapters.webhook configured in relay config.yaml
 */

import { IntakeClient } from '../src/index.js';
import type { ChatMessage } from '../src/index.js';

const RELAY_URL = process.env['RELAY_URL'] ?? 'http://localhost:8080';
const WIDGET_VERSION = '0.1.0-smoke';

async function main(): Promise<void> {
  console.log(`[smoke] connecting to relay at ${RELAY_URL}`);

  const client = new IntakeClient({
    relayUrl: RELAY_URL,
    widgetVersion: WIDGET_VERSION,
    appContext: { smoke: true },
  });

  // 1. Init
  console.log('[smoke] POST /v1/intake/init ...');
  const initResult = await client.init();
  console.log(`[smoke] session_id: ${initResult.session_id}`);
  console.log(`[smoke] capabilities:`, initResult.capabilities);

  // 2. Turn — single user message; stream deltas to stdout
  const messages: ChatMessage[] = [
    {
      role: 'user',
      content:
        'I found a bug: clicking the Save button twice submits the form twice. ' +
        'This is reproducible on Chrome 124 on macOS.',
    },
  ];

  console.log('\n[smoke] POST /v1/intake/turn — streaming ...');
  process.stdout.write('[assistant] ');

  const tokenCounts = await client.turn(messages, (delta) => {
    process.stdout.write(delta);
  });

  process.stdout.write('\n');
  console.log(
    `\n[smoke] turn complete. input_tokens=${tokenCounts.input_tokens} output_tokens=${tokenCounts.output_tokens}`
  );

  // Build the full conversation for submit (user + assistant reply)
  // The relay is stateless — we own the history and send it back
  // In a real widget, the assistant's content would be accumulated from deltas.
  // For the smoke we send just the user turn; the relay will classify from that.
  const submitMessages: ChatMessage[] = [
    ...messages,
    // We don't have the full assistant text here (we streamed it),
    // so we include a placeholder that signals end-of-conversation to the classifier.
    {
      role: 'assistant',
      content: '(end of guided conversation — smoke test)',
    },
  ];

  // 3. Submit
  console.log('\n[smoke] POST /v1/intake/submit ...');
  const submitResult = await client.submit(submitMessages);

  console.log('[smoke] SubmitResult:');
  console.log(JSON.stringify(submitResult, null, 2));

  console.log('\n[smoke] PASS');
}

main().catch((err: unknown) => {
  console.error('[smoke] FAIL:', err);
  process.exit(1);
});
