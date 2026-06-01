/**
 * Provider-agnostic 5-turn smoke driver for @intake/core.
 *
 * Drives init() then 5 sequential turn() calls, accumulating the conversation
 * history across turns (user message + synthesized assistant response from
 * streamed deltas). Prints streamed deltas and per-turn token counts. Exits
 * non-zero on any failure.
 *
 * Works against whatever provider the relay is configured with — the Phase-2
 * final smoke points the relay at each provider in turn.
 *
 * Does NOT call submit(), so no browser-global stubs (window/navigator/document)
 * are required. init() and turn() are SSR-safe in @intake/core.
 *
 * Usage:
 *   RELAY_URL=http://localhost:8080 npx tsx core/smoke/drive-multi.ts
 *
 * Prerequisites:
 *   - Relay running with a configured provider (any of anthropic/openai/gemini/ollama)
 *   - Provider's secret exported in the relay's environment
 */

import { IntakeClient } from '../src/index.js';
import type { ChatMessage } from '../src/index.js';

const RELAY_URL = process.env['RELAY_URL'] ?? 'http://localhost:8080';
const WIDGET_VERSION = '0.1.0-multi-smoke';

// The 5 user turns to send. Each builds naturally on the previous so the
// conversation is coherent and the provider must attend to history.
const USER_TURNS: string[] = [
  'I found a bug: clicking the Save button twice submits the form twice. ' +
    'This is reproducible on Chrome 124 on macOS.',
  'It also happens on Firefox 125 on Windows. ' +
    'The button does not disable itself after the first click.',
  'Looking at the network tab, I see two identical POST /api/items requests ' +
    'with the same payload within milliseconds of each other.',
  'Could this be a debounce issue? The Save button has no debounce logic ' +
    'in our current implementation.',
  'Please summarise the issue and suggest a fix in plain English.',
];

async function main(): Promise<void> {
  console.log(`[drive-multi] connecting to relay at ${RELAY_URL}`);

  const client = new IntakeClient({
    relayUrl: RELAY_URL,
    widgetVersion: WIDGET_VERSION,
    appContext: { smoke: true, driver: 'drive-multi' },
  });

  // 1. Init — establishes the session.
  console.log('[drive-multi] POST /v1/intake/init ...');
  const initResult = await client.init();
  console.log(`[drive-multi] session_id: ${initResult.session_id}`);

  // Conversation history accumulates across all turns.
  // Format: alternating user/assistant ChatMessage entries.
  // The relay expects the FULL history on each /turn call.
  const history: ChatMessage[] = [];

  // 2. Drive 5 turns.
  for (let i = 0; i < USER_TURNS.length; i++) {
    const userContent = USER_TURNS[i];
    console.log(`\n[drive-multi] --- Turn ${i + 1} / ${USER_TURNS.length} ---`);
    console.log(`[user] ${userContent}`);

    // Append the user message before calling turn().
    history.push({ role: 'user', content: userContent });

    // Collect all delta strings so we can synthesize the assistant message.
    const deltaChunks: string[] = [];

    process.stdout.write('[assistant] ');
    const tokenCounts = await client.turn(history, (delta: string) => {
      process.stdout.write(delta);
      deltaChunks.push(delta);
    });
    process.stdout.write('\n');

    // Validate that we received meaningful usage data.
    if (tokenCounts.input_tokens <= 0) {
      throw new Error(`Turn ${i + 1}: expected input_tokens > 0, got ${tokenCounts.input_tokens}`);
    }
    if (tokenCounts.output_tokens <= 0) {
      throw new Error(
        `Turn ${i + 1}: expected output_tokens > 0, got ${tokenCounts.output_tokens}`,
      );
    }

    console.log(
      `[drive-multi] turn ${i + 1} complete: ` +
        `input_tokens=${tokenCounts.input_tokens} ` +
        `output_tokens=${tokenCounts.output_tokens}`,
    );

    // Synthesize the assistant message from accumulated deltas and append to history.
    // This is how a real widget reconstructs the assistant's full response from a stream.
    const assistantContent = deltaChunks.join('');
    if (assistantContent.length === 0) {
      throw new Error(`Turn ${i + 1}: received zero delta text from assistant`);
    }
    history.push({ role: 'assistant', content: assistantContent });
  }

  console.log('\n[drive-multi] PASS — 5 turns completed successfully');
}

main().catch((err: unknown) => {
  console.error('[drive-multi] FAIL:', err);
  process.exit(1);
});
