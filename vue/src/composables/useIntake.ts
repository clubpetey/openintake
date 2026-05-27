import { ref } from 'vue';
import { IntakeClient } from '@intake/core';
import type { IntakeConfig, ChatMessage, SubmitResult } from '@intake/core';

// Security invariant: the widget NEVER handles provider API keys.
// It only calls the relay through @intake/core's IntakeClient.
// No code path in this file contacts Anthropic or any LLM provider directly.

export interface UseIntakeOptions {
  relayUrl: string;
  widgetVersion?: string;
  appContext?: Record<string, unknown>;
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

  async function start() {
    error.value = null;
    try {
      return await client.init();
    } catch (e) {
      error.value = "Couldn't connect. Please try again.";
      throw e;
    }
  }

  async function sendTurn(text: string) {
    // Append user message
    messages.value = [...messages.value, { role: 'user', content: text }];

    // Add a placeholder assistant message we will stream into
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
      error.value = e instanceof Error ? e.message : String(e);
      // Remove the empty assistant placeholder left by a failed turn
      const last = messages.value[messages.value.length - 1];
      if (last && last.role === 'assistant' && last.content === '') {
        messages.value = messages.value.slice(0, -1);
      }
    } finally {
      streaming.value = false;
    }
  }

  async function submit(routingHint?: string) {
    submitting.value = true;
    error.value = null;
    try {
      result.value = await client.submit(messages.value, routingHint);
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
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
    start,
    sendTurn,
    submit,
  };
}
