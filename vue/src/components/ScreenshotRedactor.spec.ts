import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { mount } from '@vue/test-utils';
import { nextTick } from 'vue';
import ScreenshotRedactor from './ScreenshotRedactor.vue';

// Stub HTMLCanvasElement.prototype.getContext to return a fake 2d ctx that
// records calls and yields a deterministic toDataURL.
function installCanvasStub() {
  const ctx = {
    drawImage: vi.fn(),
    fillRect: vi.fn(),
    clearRect: vi.fn(),
    strokeRect: vi.fn(),
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 1,
    globalAlpha: 1,
    setLineDash: vi.fn(),
  };
  const realGet = HTMLCanvasElement.prototype.getContext;
  HTMLCanvasElement.prototype.getContext = (function () {
    return ctx as unknown as CanvasRenderingContext2D;
  }) as unknown as typeof HTMLCanvasElement.prototype.getContext;

  const realToDataURL = HTMLCanvasElement.prototype.toDataURL;
  const toDataURLSpy = vi.fn().mockReturnValue('data:image/png;base64,REDACTED');
  HTMLCanvasElement.prototype.toDataURL = toDataURLSpy as unknown as typeof HTMLCanvasElement.prototype.toDataURL;

  return {
    ctx,
    toDataURLSpy,
    restore() {
      HTMLCanvasElement.prototype.getContext = realGet;
      HTMLCanvasElement.prototype.toDataURL = realToDataURL;
    },
  };
}

function makeSourceCanvas(width = 400, height = 300): HTMLCanvasElement {
  const c = document.createElement('canvas');
  c.width = width;
  c.height = height;
  return c;
}

describe('ScreenshotRedactor', () => {
  let stub: ReturnType<typeof installCanvasStub>;

  beforeEach(() => {
    stub = installCanvasStub();
  });

  afterEach(() => {
    stub.restore();
  });

  it('does not render when source is null', () => {
    const wrapper = mount(ScreenshotRedactor, { props: { source: null } });
    expect(wrapper.find('[data-testid="redactor-modal"]').exists()).toBe(false);
  });

  it('renders the modal when source is a canvas', () => {
    const wrapper = mount(ScreenshotRedactor, { props: { source: makeSourceCanvas() } });
    expect(wrapper.find('[data-testid="redactor-modal"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="redactor-save"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="redactor-cancel"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="redactor-clear"]').exists()).toBe(true);
  });

  it('mousedown → mousemove → mouseup draws one rectangle (fillRect called on save)', async () => {
    const wrapper = mount(ScreenshotRedactor, { props: { source: makeSourceCanvas(400, 300) } });
    const canvas = wrapper.find<HTMLCanvasElement>('[data-testid="redactor-canvas"]').element;
    canvas.dispatchEvent(new MouseEvent('mousedown', { clientX: 10, clientY: 10 }));
    canvas.dispatchEvent(new MouseEvent('mousemove', { clientX: 60, clientY: 50 }));
    canvas.dispatchEvent(new MouseEvent('mouseup', { clientX: 60, clientY: 50 }));
    await nextTick();
    // Save flattens overlay onto a copy of source.
    await wrapper.find('[data-testid="redactor-save"]').trigger('click');
    // Save path uses an offscreen flatten canvas; on it, fillRect must run at
    // least once for our single rectangle.
    expect(stub.ctx.fillRect).toHaveBeenCalled();
  });

  it('Clear All button empties the rectangle array', async () => {
    const wrapper = mount(ScreenshotRedactor, { props: { source: makeSourceCanvas(400, 300) } });
    const canvas = wrapper.find<HTMLCanvasElement>('[data-testid="redactor-canvas"]').element;
    canvas.dispatchEvent(new MouseEvent('mousedown', { clientX: 10, clientY: 10 }));
    canvas.dispatchEvent(new MouseEvent('mousemove', { clientX: 60, clientY: 50 }));
    canvas.dispatchEvent(new MouseEvent('mouseup', { clientX: 60, clientY: 50 }));
    await nextTick();
    await wrapper.find('[data-testid="redactor-clear"]').trigger('click');
    // After clear, save should produce a dataUrl but the offscreen ctx
    // must NOT have been asked to fillRect for the cleared rectangle.
    stub.ctx.fillRect.mockClear();
    await wrapper.find('[data-testid="redactor-save"]').trigger('click');
    expect(stub.ctx.fillRect).not.toHaveBeenCalled();
  });

  it('Save emits save(dataUrl) with the flattened PNG data URL', async () => {
    const wrapper = mount(ScreenshotRedactor, { props: { source: makeSourceCanvas() } });
    await wrapper.find('[data-testid="redactor-save"]').trigger('click');
    const emitted = wrapper.emitted('save');
    expect(emitted).toBeTruthy();
    expect(emitted?.[0]?.[0]).toBe('data:image/png;base64,REDACTED');
  });

  it('Cancel emits cancel and does NOT call toDataURL', async () => {
    const wrapper = mount(ScreenshotRedactor, { props: { source: makeSourceCanvas() } });
    stub.toDataURLSpy.mockClear();
    await wrapper.find('[data-testid="redactor-cancel"]').trigger('click');
    expect(wrapper.emitted('cancel')).toBeTruthy();
    expect(wrapper.emitted('save')).toBeUndefined();
    expect(stub.toDataURLSpy).not.toHaveBeenCalled();
  });

  it('ESC key cancels (emits cancel)', async () => {
    const wrapper = mount(ScreenshotRedactor, { props: { source: makeSourceCanvas() }, attachTo: document.body });
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await nextTick();
    expect(wrapper.emitted('cancel')).toBeTruthy();
    wrapper.unmount();
  });

  it('rectangles outside the source canvas bounds are clamped to source dimensions', async () => {
    const wrapper = mount(ScreenshotRedactor, { props: { source: makeSourceCanvas(100, 100) } });
    const canvas = wrapper.find<HTMLCanvasElement>('[data-testid="redactor-canvas"]').element;
    // Draw a rectangle that overflows the source on the right + bottom.
    canvas.dispatchEvent(new MouseEvent('mousedown', { clientX: 80, clientY: 80 }));
    canvas.dispatchEvent(new MouseEvent('mousemove', { clientX: 200, clientY: 200 }));
    canvas.dispatchEvent(new MouseEvent('mouseup', { clientX: 200, clientY: 200 }));
    await nextTick();
    await wrapper.find('[data-testid="redactor-save"]').trigger('click');
    // The last fillRect call args must be clamped: x+w ≤ 100, y+h ≤ 100.
    const calls = stub.ctx.fillRect.mock.calls;
    const last = calls[calls.length - 1] as [number, number, number, number];
    expect(last[0] + last[2]).toBeLessThanOrEqual(100);
    expect(last[1] + last[3]).toBeLessThanOrEqual(100);
  });

  it('focuses the Save button on open (basic focus trap entry)', async () => {
    const wrapper = mount(ScreenshotRedactor, { props: { source: makeSourceCanvas() }, attachTo: document.body });
    await nextTick();
    const saveBtn = wrapper.find<HTMLButtonElement>('[data-testid="redactor-save"]').element;
    expect(document.activeElement).toBe(saveBtn);
    wrapper.unmount();
  });
});
