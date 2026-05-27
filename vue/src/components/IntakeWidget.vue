<script setup lang="ts">
// Security invariant: this widget NEVER handles provider API keys.
// It only calls the relay through @intake/core's IntakeClient.
// All LLM calls happen inside the relay process — never from this browser widget.

import { ref, onMounted } from 'vue';
import ConversationView from './ConversationView.vue';
import { useIntake } from '../composables/useIntake';

const props = defineProps<{
  relayUrl: string;
  appContext?: Record<string, unknown>;
}>();

const isOpen = ref(false);
const inputText = ref('');

const { messages, streaming, submitting, result, error, start, sendTurn, submit } = useIntake({
  relayUrl: props.relayUrl,
  widgetVersion: '0.1.0',
  appContext: props.appContext,
});

// Initialize the session when the widget mounts
onMounted(async () => {
  try {
    await start();
  } catch {
    // Session init failed — will retry on first send
  }
});

function togglePanel() {
  isOpen.value = !isOpen.value;
}

async function handleSend() {
  const text = inputText.value.trim();
  if (!text || streaming.value) return;
  inputText.value = '';
  await sendTurn(text);
}

async function handleSubmit() {
  if (submitting.value || streaming.value) return;
  await submit();
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault();
    handleSend();
  }
}
</script>

<template>
  <div class="intake-widget">
    <!-- Launcher button -->
    <button
      class="intake-widget__launcher"
      :class="{ 'intake-widget__launcher--open': isOpen }"
      :aria-label="isOpen ? 'Close support widget' : 'Open support widget'"
      data-testid="launcher-button"
      @click="togglePanel"
    >
      <svg v-if="!isOpen" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>
      </svg>
      <svg v-else xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <line x1="18" y1="6" x2="6" y2="18"/>
        <line x1="6" y1="6" x2="18" y2="18"/>
      </svg>
    </button>

    <!-- Chat panel -->
    <div
      v-if="isOpen"
      class="intake-widget__panel"
      data-testid="chat-panel"
      role="dialog"
      aria-label="Support chat"
    >
      <!-- Header -->
      <div class="intake-widget__header">
        <span class="intake-widget__title">Report an Issue</span>
      </div>

      <!-- Result view (shown after submit) -->
      <div v-if="result" class="intake-widget__result" data-testid="submit-result">
        <div class="intake-widget__result-icon" aria-hidden="true">✓</div>
        <p class="intake-widget__result-text">Your report has been submitted.</p>
        <p class="intake-widget__result-id">
          Ticket ID: <code data-testid="external-id">{{ result.external_id }}</code>
        </p>
        <a
          v-if="result.external_url"
          :href="result.external_url"
          target="_blank"
          rel="noopener noreferrer"
          class="intake-widget__result-link"
        >
          View ticket
        </a>
      </div>

      <!-- Conversation view + input (shown before submit) -->
      <template v-else>
        <ConversationView
          :messages="messages"
          :streaming="streaming"
        />

        <!-- Error banner -->
        <div v-if="error" class="intake-widget__error" data-testid="error-banner" role="alert">
          {{ error }}
        </div>

        <!-- Input area -->
        <div class="intake-widget__input-area">
          <textarea
            v-model="inputText"
            class="intake-widget__input"
            placeholder="Describe your issue…"
            rows="2"
            :disabled="streaming || submitting"
            data-testid="message-input"
            aria-label="Message"
            @keydown="handleKeydown"
          />
          <div class="intake-widget__actions">
            <button
              class="intake-widget__btn intake-widget__btn--send"
              :disabled="!inputText.trim() || streaming"
              data-testid="send-button"
              @click="handleSend"
            >
              Send
            </button>
            <button
              class="intake-widget__btn intake-widget__btn--submit"
              :disabled="messages.length === 0 || streaming || submitting"
              data-testid="submit-button"
              @click="handleSubmit"
            >
              {{ submitting ? 'Submitting…' : 'Submit' }}
            </button>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
/* ── Launcher button ─────────────────────────────────────── */
.intake-widget {
  position: fixed;
  bottom: 24px;
  right: 24px;
  z-index: 9999;
  font-family: system-ui, sans-serif;
}

.intake-widget__launcher {
  width: 52px;
  height: 52px;
  border-radius: 50%;
  background-color: #2563eb;
  color: #fff;
  border: none;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.2);
  transition: background-color 0.2s ease, transform 0.2s ease;
}

.intake-widget__launcher:hover {
  background-color: #1d4ed8;
  transform: scale(1.05);
}

.intake-widget__launcher--open {
  background-color: #64748b;
}

/* ── Panel ───────────────────────────────────────────────── */
.intake-widget__panel {
  position: absolute;
  bottom: 64px;
  right: 0;
  width: 360px;
  height: 480px;
  background-color: #fff;
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.15);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.intake-widget__header {
  padding: 14px 16px;
  background-color: #2563eb;
  color: #fff;
  flex-shrink: 0;
}

.intake-widget__title {
  font-size: 15px;
  font-weight: 600;
}

/* ── Input area ──────────────────────────────────────────── */
.intake-widget__input-area {
  padding: 12px;
  border-top: 1px solid #e2e8f0;
  flex-shrink: 0;
}

.intake-widget__input {
  width: 100%;
  box-sizing: border-box;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  padding: 8px 10px;
  font-size: 13px;
  resize: none;
  outline: none;
  font-family: inherit;
  transition: border-color 0.15s;
}

.intake-widget__input:focus {
  border-color: #2563eb;
}

.intake-widget__input:disabled {
  background-color: #f8fafc;
  color: #94a3b8;
}

.intake-widget__actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 8px;
}

.intake-widget__btn {
  padding: 6px 14px;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  border: none;
  transition: background-color 0.15s, opacity 0.15s;
}

.intake-widget__btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.intake-widget__btn--send {
  background-color: #2563eb;
  color: #fff;
}

.intake-widget__btn--send:hover:not(:disabled) {
  background-color: #1d4ed8;
}

.intake-widget__btn--submit {
  background-color: #16a34a;
  color: #fff;
}

.intake-widget__btn--submit:hover:not(:disabled) {
  background-color: #15803d;
}

/* ── Result view ─────────────────────────────────────────── */
.intake-widget__result {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 24px;
  text-align: center;
  gap: 8px;
}

.intake-widget__result-icon {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background-color: #dcfce7;
  color: #16a34a;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24px;
  font-weight: bold;
  margin-bottom: 8px;
}

.intake-widget__result-text {
  font-size: 15px;
  font-weight: 600;
  color: #1e293b;
  margin: 0;
}

.intake-widget__result-id {
  font-size: 13px;
  color: #64748b;
  margin: 0;
}

.intake-widget__result-id code {
  font-family: ui-monospace, monospace;
  background-color: #f1f5f9;
  padding: 2px 6px;
  border-radius: 4px;
}

.intake-widget__result-link {
  font-size: 13px;
  color: #2563eb;
  text-decoration: none;
}

.intake-widget__result-link:hover {
  text-decoration: underline;
}

/* ── Error banner ────────────────────────────────────────── */
.intake-widget__error {
  margin: 0 12px;
  padding: 8px 12px;
  background-color: #fef2f2;
  color: #b91c1c;
  border-radius: 6px;
  font-size: 13px;
  flex-shrink: 0;
}
</style>
