// Shared vitest setup for vue/ tests.
//
// Phase 6-iii cleanup: previously each spec patched HTMLCanvasElement.prototype
// at module scope without restoring originals. If test-file execution order
// shifted, later specs inherited the stubs. This setup file installs a single
// shared canvas stub once (idempotent via captured originals + afterAll
// restore) so both IntakeWidget.spec.ts and ScreenshotRedactor.spec.ts share
// the same baseline.
//
// Individual tests that need per-test behavior (e.g. a specific toDataURL
// return value) may still override the prototype methods inside the test body;
// they should restore via the original references this file exposes if
// stricter isolation is required.

import { afterAll, beforeAll, vi } from 'vitest';

interface FakeCtx {
  drawImage: ReturnType<typeof vi.fn>;
  fillRect: ReturnType<typeof vi.fn>;
  clearRect: ReturnType<typeof vi.fn>;
  strokeRect: ReturnType<typeof vi.fn>;
  beginPath: ReturnType<typeof vi.fn>;
  moveTo: ReturnType<typeof vi.fn>;
  lineTo: ReturnType<typeof vi.fn>;
  stroke: ReturnType<typeof vi.fn>;
  setLineDash: ReturnType<typeof vi.fn>;
  fillStyle: string;
  strokeStyle: string;
  lineWidth: number;
  globalAlpha: number;
}

function makeFakeCtx(): FakeCtx {
  return {
    drawImage: vi.fn(),
    fillRect: vi.fn(),
    clearRect: vi.fn(),
    strokeRect: vi.fn(),
    beginPath: vi.fn(),
    moveTo: vi.fn(),
    lineTo: vi.fn(),
    stroke: vi.fn(),
    setLineDash: vi.fn(),
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 1,
    globalAlpha: 1,
  };
}

let originalGetContext: typeof HTMLCanvasElement.prototype.getContext;
let originalToDataURL: typeof HTMLCanvasElement.prototype.toDataURL;
let originalToBlob: typeof HTMLCanvasElement.prototype.toBlob;

beforeAll(() => {
  originalGetContext = HTMLCanvasElement.prototype.getContext;
  originalToDataURL = HTMLCanvasElement.prototype.toDataURL;
  originalToBlob = HTMLCanvasElement.prototype.toBlob;

  HTMLCanvasElement.prototype.getContext = function () {
    return makeFakeCtx() as unknown as CanvasRenderingContext2D;
  } as unknown as typeof HTMLCanvasElement.prototype.getContext;

  HTMLCanvasElement.prototype.toDataURL = function () {
    return 'data:image/png;base64,AAAA';
  } as unknown as typeof HTMLCanvasElement.prototype.toDataURL;

  HTMLCanvasElement.prototype.toBlob = function (cb: BlobCallback) {
    cb(new Blob([new Uint8Array([0])], { type: 'image/png' }));
  } as unknown as typeof HTMLCanvasElement.prototype.toBlob;
});

afterAll(() => {
  HTMLCanvasElement.prototype.getContext = originalGetContext;
  HTMLCanvasElement.prototype.toDataURL = originalToDataURL;
  HTMLCanvasElement.prototype.toBlob = originalToBlob;
});
