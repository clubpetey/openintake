import { describe, it, expect, afterEach } from 'vitest';
import { captureClient, capturePageMetadata } from './context.js';

// In Node 24, window/navigator/document are read-only getters on globalThis.
// We use Object.defineProperty with configurable:true to stub them per test.

function stubGlobal(name: string, value: unknown) {
  Object.defineProperty(globalThis, name, {
    value,
    writable: true,
    configurable: true,
  });
}

function deleteGlobal(name: string) {
  // Restore to undefined (simulate SSR — no browser globals)
  Object.defineProperty(globalThis, name, {
    value: undefined,
    writable: true,
    configurable: true,
  });
}

afterEach(() => {
  // Reset all three to undefined after each test so tests don't bleed into each other.
  // Node's own globalThis.navigator/document will be redefined by Node after the test file
  // completes; within the file these stubs are sufficient.
  deleteGlobal('window');
  deleteGlobal('navigator');
  deleteGlobal('document');
});

describe('captureClient — SSR (no window)', () => {
  it('returns safe defaults when window is absent', () => {
    deleteGlobal('window');
    deleteGlobal('navigator');
    deleteGlobal('document');
    const info = captureClient('0.0.1');
    expect(info.widget_version).toBe('0.0.1');
    expect(info.url).toBe('');
    expect(info.referrer).toBeNull();
    expect(info.user_agent).toBe('');
    expect(info.viewport).toEqual({ w: 0, h: 0 });
    expect(info.locale).toBe('');
  });
});

describe('captureClient — browser (window present)', () => {
  it('captures url from window.location.href', () => {
    stubGlobal('window', {
      location: { href: 'https://example.com/page' },
      innerWidth: 1280,
      innerHeight: 800,
    });
    stubGlobal('navigator', { userAgent: 'TestAgent/1.0', language: 'en-US' });
    stubGlobal('document', {
      referrer: 'https://referrer.example.com',
      title: 'Test Page',
      querySelectorAll: () => [],
    });
    const info = captureClient('1.2.3');
    expect(info.url).toBe('https://example.com/page');
  });

  it('captures referrer from document.referrer', () => {
    stubGlobal('window', {
      location: { href: 'https://example.com/page' },
      innerWidth: 1280,
      innerHeight: 800,
    });
    stubGlobal('navigator', { userAgent: 'TestAgent/1.0', language: 'en-US' });
    stubGlobal('document', {
      referrer: 'https://referrer.example.com',
      title: 'Test Page',
      querySelectorAll: () => [],
    });
    const info = captureClient('1.2.3');
    expect(info.referrer).toBe('https://referrer.example.com');
  });

  it('captures user_agent from navigator.userAgent', () => {
    stubGlobal('window', {
      location: { href: 'https://example.com/page' },
      innerWidth: 1280,
      innerHeight: 800,
    });
    stubGlobal('navigator', { userAgent: 'TestAgent/1.0', language: 'en-US' });
    stubGlobal('document', {
      referrer: '',
      title: '',
      querySelectorAll: () => [],
    });
    const info = captureClient('1.2.3');
    expect(info.user_agent).toBe('TestAgent/1.0');
  });

  it('captures viewport from window.innerWidth/innerHeight', () => {
    stubGlobal('window', {
      location: { href: 'https://example.com/page' },
      innerWidth: 1280,
      innerHeight: 800,
    });
    stubGlobal('navigator', { userAgent: 'TestAgent/1.0', language: 'en-US' });
    stubGlobal('document', {
      referrer: '',
      title: '',
      querySelectorAll: () => [],
    });
    const info = captureClient('1.2.3');
    expect(info.viewport).toEqual({ w: 1280, h: 800 });
  });

  it('captures locale from navigator.language', () => {
    stubGlobal('window', {
      location: { href: 'https://example.com/page' },
      innerWidth: 1280,
      innerHeight: 800,
    });
    stubGlobal('navigator', { userAgent: 'TestAgent/1.0', language: 'en-US' });
    stubGlobal('document', {
      referrer: '',
      title: '',
      querySelectorAll: () => [],
    });
    const info = captureClient('1.2.3');
    expect(info.locale).toBe('en-US');
  });

  it('sets referrer to null when document.referrer is empty string', () => {
    stubGlobal('window', {
      location: { href: 'https://example.com/' },
      innerWidth: 800,
      innerHeight: 600,
    });
    stubGlobal('navigator', { userAgent: 'UA', language: 'fr-FR' });
    stubGlobal('document', {
      referrer: '',
      title: '',
      querySelectorAll: () => [],
    });
    const info = captureClient('0.1.0');
    expect(info.referrer).toBeNull();
  });
});

describe('capturePageMetadata', () => {
  it('returns empty record when window is absent', () => {
    deleteGlobal('window');
    deleteGlobal('document');
    deleteGlobal('navigator');
    const meta = capturePageMetadata();
    expect(meta).toEqual({});
  });

  it('captures document.title', () => {
    stubGlobal('window', {});
    stubGlobal('navigator', {});
    stubGlobal('document', {
      title: 'My Page',
      querySelectorAll: () => [],
    });
    const meta = capturePageMetadata();
    expect(meta['title']).toBe('My Page');
  });

  it('captures og:title and og:description meta tags', () => {
    const mockMeta = [
      {
        getAttribute: (k: string) => {
          if (k === 'property') return 'og:title';
          if (k === 'content') return 'OG Title';
          return null;
        },
      },
      {
        getAttribute: (k: string) => {
          if (k === 'property') return 'og:description';
          if (k === 'content') return 'OG Desc';
          return null;
        },
      },
    ];
    stubGlobal('window', {});
    stubGlobal('navigator', {});
    stubGlobal('document', {
      title: '',
      querySelectorAll: () => ({
        forEach: (cb: (el: (typeof mockMeta)[0]) => void) =>
          mockMeta.forEach(cb),
      }),
    });
    const meta = capturePageMetadata();
    expect(meta['og:title']).toBe('OG Title');
    expect(meta['og:description']).toBe('OG Desc');
  });
});
