import type { ClientInfo, Viewport } from './types.js';

/**
 * Captures browser client context for inclusion in SubmitRequest.client.
 * SSR-safe: all window/navigator/document accesses are guarded.
 */
export function captureClient(widgetVersion: string): ClientInfo {
  if (typeof window === 'undefined') {
    return {
      widget_version: widgetVersion,
      url: '',
      referrer: null,
      user_agent: '',
      viewport: { w: 0, h: 0 },
      locale: '',
    };
  }

  const viewport: Viewport = {
    w: window.innerWidth,
    h: window.innerHeight,
  };

  const referrerRaw = typeof document !== 'undefined' ? document.referrer : '';
  const referrer = referrerRaw.length > 0 ? referrerRaw : null;

  return {
    widget_version: widgetVersion,
    url: window.location.href,
    referrer,
    user_agent: typeof navigator !== 'undefined' ? navigator.userAgent : '',
    viewport,
    locale: typeof navigator !== 'undefined' ? navigator.language : '',
  };
}

/**
 * Captures Open Graph and title metadata from the current page.
 * SSR-safe: returns empty record when document is unavailable.
 */
export function capturePageMetadata(): Record<string, unknown> {
  if (typeof window === 'undefined' || typeof document === 'undefined') {
    return {};
  }

  const meta: Record<string, unknown> = {};

  if (document.title) {
    meta['title'] = document.title;
  }

  const metaTags = document.querySelectorAll('meta[property]');
  metaTags.forEach((el) => {
    const property = el.getAttribute('property');
    const content = el.getAttribute('content');
    if (property && content !== null) {
      meta[property] = content;
    }
  });

  return meta;
}
