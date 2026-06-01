import { describe, it, expect, beforeEach, beforeAll, vi } from 'vitest';
import { setHtml2Canvas, capturePage, canvasToDataURL, __resetCaptureForTests } from './capture.js';

// L004: capture.ts references document.body; the core test env is Node.
// Stub a minimal `document` global with a sentinel body, using
// Object.defineProperty (plain assignment to read-only globalThis props throws
// in Node 24 — same shape as the navigator stub in core/smoke/drive.ts).
beforeAll(() => {
  if (typeof (globalThis as { document?: unknown }).document === 'undefined') {
    Object.defineProperty(globalThis, 'document', {
      value: { body: { __sentinel: 'document.body' } },
      configurable: true,
      writable: true,
    });
  }
});

beforeEach(() => {
  __resetCaptureForTests();
});

describe('setHtml2Canvas / capturePage', () => {
  it('capturePage() invokes the injected html2canvas with document.body', async () => {
    const fakeCanvas = { width: 100, height: 50 } as unknown as HTMLCanvasElement;
    const stub = vi.fn().mockResolvedValue(fakeCanvas);
    setHtml2Canvas(stub);

    const canvas = await capturePage();

    expect(stub).toHaveBeenCalledOnce();
    expect(stub).toHaveBeenCalledWith(document.body, expect.any(Object));
    expect(canvas).toBe(fakeCanvas);
  });

  it('capturePage() rejects when no html2canvas has been registered and no auto-loader is available', async () => {
    // No setHtml2Canvas() call; production would dynamic-import. Tests must
    // register a stub; this test asserts the explicit-failure surface.
    await expect(capturePage()).rejects.toThrow(/html2canvas not registered/i);
  });

  it('capturePage() rejects on a 0x0 canvas', async () => {
    const fakeCanvas = { width: 0, height: 0 } as unknown as HTMLCanvasElement;
    setHtml2Canvas(vi.fn().mockResolvedValue(fakeCanvas));
    await expect(capturePage()).rejects.toThrow(/0x0/);
  });
});

describe('canvasToDataURL', () => {
  it('converts a canvas to a data: URL via toBlob + FileReader', async () => {
    const blob = new Blob([new Uint8Array([1, 2, 3])], { type: 'image/png' });
    const fakeCanvas = {
      width: 10,
      height: 10,
      toBlob(cb: (b: Blob | null) => void, mime: string) {
        expect(mime).toBe('image/png');
        cb(blob);
      },
    } as unknown as HTMLCanvasElement;

    // Stub FileReader to deterministically yield a known data URL
    const expected = 'data:image/png;base64,AQID';
    class FakeReader {
      result: string | null = null;
      onload: (() => void) | null = null;
      onerror: (() => void) | null = null;
      readAsDataURL(_b: Blob) {
        this.result = expected;
        queueMicrotask(() => this.onload && this.onload());
      }
    }
    const realFR = globalThis.FileReader;
    (globalThis as unknown as { FileReader: typeof FileReader }).FileReader =
      FakeReader as unknown as typeof FileReader;

    try {
      const url = await canvasToDataURL(fakeCanvas, 'image/png');
      expect(url).toBe(expected);
    } finally {
      (globalThis as unknown as { FileReader: typeof FileReader }).FileReader = realFR;
    }
  });

  it('rejects when toBlob yields null', async () => {
    const fakeCanvas = {
      width: 10,
      height: 10,
      toBlob(cb: (b: Blob | null) => void) {
        cb(null);
      },
    } as unknown as HTMLCanvasElement;
    await expect(canvasToDataURL(fakeCanvas, 'image/png')).rejects.toThrow(/toBlob returned null/);
  });

  it('rejects when FileReader errors', async () => {
    const blob = new Blob([new Uint8Array([1])], { type: 'image/png' });
    const fakeCanvas = {
      width: 10,
      height: 10,
      toBlob(cb: (b: Blob | null) => void) {
        cb(blob);
      },
    } as unknown as HTMLCanvasElement;
    class FailingReader {
      result: string | null = null;
      onload: (() => void) | null = null;
      onerror: (() => void) | null = null;
      readAsDataURL(_b: Blob) {
        queueMicrotask(() => this.onerror && this.onerror());
      }
    }
    const realFR = globalThis.FileReader;
    (globalThis as unknown as { FileReader: typeof FileReader }).FileReader =
      FailingReader as unknown as typeof FileReader;
    try {
      await expect(canvasToDataURL(fakeCanvas, 'image/png')).rejects.toThrow(/FileReader/);
    } finally {
      (globalThis as unknown as { FileReader: typeof FileReader }).FileReader = realFR;
    }
  });
});
