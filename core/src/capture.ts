// Phase 6 §3.5 — html2canvas is dependency-injected so tests do not load the
// real library. Production calls setHtml2Canvas(real) once on first capture
// via a dynamic import; tests call setHtml2Canvas(stub) before capturePage().

export type Html2CanvasFn = (
  el: HTMLElement,
  opts?: Record<string, unknown>,
) => Promise<HTMLCanvasElement>;

let registered: Html2CanvasFn | null = null;

/**
 * Register the html2canvas implementation. Production callers invoke this
 * once during widget bootstrap with the real `html2canvas` module's default
 * export; tests invoke it with a stub before calling `capturePage()`.
 */
export function setHtml2Canvas(fn: Html2CanvasFn): void {
  registered = fn;
}

/**
 * TEST-ONLY helper. Resets the registered html2canvas so each test starts
 * from a known clean state. Production never calls this.
 */
export function __resetCaptureForTests(): void {
  registered = null;
}

/**
 * Captures `document.body` to a canvas via the registered html2canvas.
 * Throws if no implementation has been registered or if the resulting
 * canvas is 0x0 (defensive — a 0x0 canvas cannot be turned into a useful
 * data URL and would mislead the redactor modal).
 */
export async function capturePage(): Promise<HTMLCanvasElement> {
  if (registered === null) {
    throw new Error(
      'capture: html2canvas not registered; call setHtml2Canvas(fn) first',
    );
  }
  const canvas = await registered(document.body, {
    // Conservative defaults — tests pass an opts object through but do not
    // depend on its contents.
    useCORS: true,
    backgroundColor: null,
  });
  if (canvas.width === 0 || canvas.height === 0) {
    throw new Error('capture: refusing 0x0 canvas');
  }
  return canvas;
}

/**
 * Converts a canvas to a `data:` URL of the requested MIME via canvas.toBlob
 * + FileReader.readAsDataURL. Promise-wraps the callback APIs and rejects on
 * either toBlob yielding null or FileReader erroring.
 */
export function canvasToDataURL(
  canvas: HTMLCanvasElement,
  mime: 'image/png' | 'image/jpeg' | 'image/webp',
): Promise<string> {
  return new Promise<string>((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (blob === null) {
        reject(new Error('canvasToDataURL: toBlob returned null'));
        return;
      }
      const reader = new FileReader();
      reader.onload = () => {
        const r = reader.result;
        if (typeof r === 'string') {
          resolve(r);
        } else {
          reject(new Error('canvasToDataURL: FileReader yielded non-string result'));
        }
      };
      reader.onerror = () => {
        reject(new Error('canvasToDataURL: FileReader errored'));
      };
      reader.readAsDataURL(blob);
    }, mime);
  });
}
