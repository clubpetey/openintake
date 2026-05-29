<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount, nextTick } from 'vue';

interface Rect { x: number; y: number; w: number; h: number }

const props = defineProps<{
  source: HTMLCanvasElement | null;
}>();

const emit = defineEmits<{
  (e: 'save', dataUrl: string): void;
  (e: 'cancel'): void;
}>();

const overlayRef = ref<HTMLCanvasElement | null>(null);
const saveBtnRef = ref<HTMLButtonElement | null>(null);
const modalRef = ref<HTMLDivElement | null>(null);

const rects = ref<Rect[]>([]);
const dragging = ref(false);
const dragStart = ref<{ x: number; y: number } | null>(null);
const dragCurrent = ref<{ x: number; y: number } | null>(null);

function clamp(v: number, lo: number, hi: number): number {
  return Math.min(Math.max(v, lo), hi);
}

function clampRect(r: Rect, w: number, h: number): Rect {
  const x1 = clamp(r.x, 0, w);
  const y1 = clamp(r.y, 0, h);
  const x2 = clamp(r.x + r.w, 0, w);
  const y2 = clamp(r.y + r.h, 0, h);
  return { x: Math.min(x1, x2), y: Math.min(y1, y2), w: Math.abs(x2 - x1), h: Math.abs(y2 - y1) };
}

function repaintOverlay() {
  const overlay = overlayRef.value;
  const src = props.source;
  if (!overlay || !src) return;
  const ctx = overlay.getContext('2d');
  if (!ctx) return;
  ctx.clearRect(0, 0, overlay.width, overlay.height);
  // Draw the source image as a backdrop the user is annotating over.
  ctx.drawImage(src, 0, 0);
  // Solid black redaction overlays.
  ctx.fillStyle = '#000';
  for (const r of rects.value) {
    const c = clampRect(r, src.width, src.height);
    ctx.fillRect(c.x, c.y, c.w, c.h);
  }
  // In-progress drag preview (dashed outline).
  if (dragging.value && dragStart.value && dragCurrent.value) {
    const live: Rect = {
      x: Math.min(dragStart.value.x, dragCurrent.value.x),
      y: Math.min(dragStart.value.y, dragCurrent.value.y),
      w: Math.abs(dragCurrent.value.x - dragStart.value.x),
      h: Math.abs(dragCurrent.value.y - dragStart.value.y),
    };
    ctx.strokeStyle = '#000';
    ctx.lineWidth = 1;
    ctx.setLineDash([4, 4]);
    ctx.strokeRect(live.x, live.y, live.w, live.h);
    ctx.setLineDash([]);
  }
}

function onMouseDown(ev: MouseEvent) {
  const overlay = overlayRef.value;
  if (!overlay) return;
  const rect = overlay.getBoundingClientRect();
  const x = ev.clientX - rect.left;
  const y = ev.clientY - rect.top;
  dragging.value = true;
  dragStart.value = { x, y };
  dragCurrent.value = { x, y };
}

function onMouseMove(ev: MouseEvent) {
  if (!dragging.value) return;
  const overlay = overlayRef.value;
  if (!overlay) return;
  const rect = overlay.getBoundingClientRect();
  dragCurrent.value = { x: ev.clientX - rect.left, y: ev.clientY - rect.top };
  repaintOverlay();
}

function onMouseUp(ev: MouseEvent) {
  if (!dragging.value || !dragStart.value) return;
  const overlay = overlayRef.value;
  if (!overlay) return;
  const rect = overlay.getBoundingClientRect();
  const endX = ev.clientX - rect.left;
  const endY = ev.clientY - rect.top;
  const r: Rect = {
    x: Math.min(dragStart.value.x, endX),
    y: Math.min(dragStart.value.y, endY),
    w: Math.abs(endX - dragStart.value.x),
    h: Math.abs(endY - dragStart.value.y),
  };
  if (r.w > 0 && r.h > 0) {
    rects.value = [...rects.value, r];
  }
  dragging.value = false;
  dragStart.value = null;
  dragCurrent.value = null;
  repaintOverlay();
}

function onClearAll() {
  rects.value = [];
  repaintOverlay();
}

function onSave() {
  const src = props.source;
  if (!src) {
    emit('cancel');
    return;
  }
  // Flatten redactions onto an offscreen copy of source, then emit data URL.
  const out = document.createElement('canvas');
  out.width = src.width;
  out.height = src.height;
  const ctx = out.getContext('2d');
  if (!ctx) {
    emit('cancel');
    return;
  }
  ctx.drawImage(src, 0, 0);
  ctx.fillStyle = '#000';
  for (const r of rects.value) {
    const c = clampRect(r, src.width, src.height);
    if (c.w > 0 && c.h > 0) {
      ctx.fillRect(c.x, c.y, c.w, c.h);
    }
  }
  emit('save', out.toDataURL('image/png'));
}

function onCancel() {
  emit('cancel');
}

function onKeyDown(ev: KeyboardEvent) {
  if (ev.key === 'Escape') {
    ev.preventDefault();
    emit('cancel');
    return;
  }
  // Basic focus trap: keep Tab inside the modal.
  if (ev.key === 'Tab' && modalRef.value) {
    const focusables = modalRef.value.querySelectorAll<HTMLElement>(
      'button, [tabindex]:not([tabindex="-1"])',
    );
    if (focusables.length === 0) return;
    const first = focusables[0];
    const last = focusables[focusables.length - 1];
    if (ev.shiftKey && document.activeElement === first) {
      ev.preventDefault();
      last.focus();
    } else if (!ev.shiftKey && document.activeElement === last) {
      ev.preventDefault();
      first.focus();
    }
  }
}

watch(
  () => props.source,
  async (src) => {
    if (!src) return;
    rects.value = [];
    await nextTick();
    const overlay = overlayRef.value;
    if (overlay) {
      overlay.width = src.width;
      overlay.height = src.height;
      repaintOverlay();
    }
    saveBtnRef.value?.focus();
  },
  { immediate: true },
);

onMounted(() => {
  document.addEventListener('keydown', onKeyDown);
});

onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKeyDown);
});
</script>

<template>
  <div
    v-if="source !== null"
    ref="modalRef"
    class="redactor"
    role="dialog"
    aria-modal="true"
    aria-label="Redact screenshot"
    data-testid="redactor-modal"
  >
    <div class="redactor__backdrop" @click="onCancel" />
    <div class="redactor__panel" @click.stop>
      <div class="redactor__header">
        <span class="redactor__title">Redact screenshot</span>
        <span class="redactor__hint">Drag to draw black boxes over sensitive areas.</span>
      </div>
      <div class="redactor__canvas-wrap">
        <canvas
          ref="overlayRef"
          class="redactor__canvas"
          data-testid="redactor-canvas"
          @mousedown="onMouseDown"
          @mousemove="onMouseMove"
          @mouseup="onMouseUp"
        />
      </div>
      <div class="redactor__actions">
        <button
          type="button"
          class="redactor__btn redactor__btn--ghost"
          data-testid="redactor-clear"
          @click="onClearAll"
        >
          Clear all
        </button>
        <button
          type="button"
          class="redactor__btn redactor__btn--ghost"
          data-testid="redactor-cancel"
          @click="onCancel"
        >
          Cancel
        </button>
        <button
          ref="saveBtnRef"
          type="button"
          class="redactor__btn redactor__btn--primary"
          data-testid="redactor-save"
          @click="onSave"
        >
          Save
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.redactor {
  position: fixed;
  inset: 0;
  z-index: 100000;
  display: flex;
  align-items: center;
  justify-content: center;
  font-family: system-ui, sans-serif;
}
.redactor__backdrop {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
}
.redactor__panel {
  position: relative;
  background: #fff;
  border-radius: 8px;
  padding: 16px;
  max-width: 90vw;
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  gap: 12px;
  box-shadow: 0 10px 40px rgba(0, 0, 0, 0.3);
}
.redactor__header {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.redactor__title {
  font-size: 15px;
  font-weight: 600;
  color: #0f172a;
}
.redactor__hint {
  font-size: 12px;
  color: #64748b;
}
.redactor__canvas-wrap {
  overflow: auto;
  max-height: 70vh;
  background: #f1f5f9;
  border: 1px solid #e2e8f0;
}
.redactor__canvas {
  display: block;
  cursor: crosshair;
}
.redactor__actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
.redactor__btn {
  padding: 6px 14px;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  border: 1px solid transparent;
}
.redactor__btn--ghost {
  background: #fff;
  color: #334155;
  border-color: #cbd5e1;
}
.redactor__btn--primary {
  background: #2563eb;
  color: #fff;
}
</style>
