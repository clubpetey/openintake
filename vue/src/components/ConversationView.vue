<script setup lang="ts">
import type { ChatMessage } from '@intake/core';

defineProps<{
  messages: ChatMessage[];
  streaming: boolean;
}>();
</script>

<template>
  <div class="conversation-view">
    <div
      v-for="(msg, i) in messages"
      :key="i"
      :class="['message', `message--${msg.role}`]"
      data-testid="message"
    >
      <span class="message__content">{{ msg.content }}</span>
    </div>
    <div
      v-if="streaming"
      class="streaming-indicator"
      data-testid="streaming-indicator"
      aria-label="Assistant is typing"
    >
      <span class="streaming-indicator__dot" />
      <span class="streaming-indicator__dot" />
      <span class="streaming-indicator__dot" />
    </div>
  </div>
</template>

<style scoped>
.conversation-view {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  overflow-y: auto;
  flex: 1;
}

.message {
  max-width: 80%;
  padding: 8px 12px;
  border-radius: 8px;
  word-break: break-word;
  font-size: 14px;
  line-height: 1.5;
}

.message--user {
  align-self: flex-end;
  background-color: #2563eb;
  color: #fff;
}

.message--assistant {
  align-self: flex-start;
  background-color: #f1f5f9;
  color: #1e293b;
}

.message__content {
  white-space: pre-wrap;
}

.streaming-indicator {
  display: flex;
  gap: 4px;
  align-self: flex-start;
  padding: 8px 12px;
}

.streaming-indicator__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background-color: #94a3b8;
  animation: pulse 1s ease-in-out infinite;
}

.streaming-indicator__dot:nth-child(2) {
  animation-delay: 0.2s;
}

.streaming-indicator__dot:nth-child(3) {
  animation-delay: 0.4s;
}

@keyframes pulse {
  0%, 100% { opacity: 0.3; }
  50% { opacity: 1; }
}
</style>
