<script setup lang="ts">
import { computed } from 'vue';
import type { PendingAttachment } from '@openintake/core';

const props = defineProps<{
  items: PendingAttachment[];
  maxTotalBytes: number;
}>();

const emit = defineEmits<{
  (e: 'remove', index: number): void;
}>();

const aggregate = computed(() => props.items.reduce((s, a) => s + a.sizeBytes, 0));

function humanBytes(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_048_576).toFixed(1)} MB`;
  if (n >= 1_000) return `${(n / 1_024).toFixed(1)} KB`;
  return `${n} B`;
}

const badgeText = computed(
  () => `${humanBytes(aggregate.value)} / ${humanBytes(props.maxTotalBytes)}`,
);

function onRemove(i: number) {
  emit('remove', i);
}
</script>

<template>
  <div v-if="items.length > 0" class="strip" data-testid="attachment-strip">
    <ul class="strip__list">
      <li v-for="(att, i) in items" :key="i" class="strip__item" data-testid="attachment-thumb">
        <img :src="att.dataUrl" :alt="att.label ?? `screenshot ${i + 1}`" class="strip__thumb" />
        <button
          type="button"
          class="strip__remove"
          :aria-label="`Remove attachment ${i + 1}`"
          data-testid="attachment-remove"
          @click="onRemove(i)"
        >
          ×
        </button>
      </li>
    </ul>
    <div class="strip__badge" data-testid="attachment-aggregate">
      {{ badgeText }}
    </div>
  </div>
</template>

<style scoped>
.strip {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border-top: 1px solid #e2e8f0;
  background: #f8fafc;
  flex-shrink: 0;
}
.strip__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  gap: 6px;
  overflow-x: auto;
}
.strip__item {
  position: relative;
  width: 48px;
  height: 48px;
  border-radius: 4px;
  overflow: hidden;
  border: 1px solid #cbd5e1;
  flex-shrink: 0;
}
.strip__thumb {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
.strip__remove {
  position: absolute;
  top: 2px;
  right: 2px;
  width: 18px;
  height: 18px;
  border-radius: 9px;
  border: none;
  background: rgba(15, 23, 42, 0.75);
  color: #fff;
  font-size: 14px;
  line-height: 1;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
}
.strip__badge {
  font-size: 12px;
  color: #475569;
  margin-left: 12px;
  flex-shrink: 0;
}
</style>
