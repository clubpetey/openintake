import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mount, flushPromises } from '@vue/test-utils';
import IntakeWidget from './IntakeWidget.vue';

const mockInit = vi.fn();
const mockTurn = vi.fn();
const mockSubmit = vi.fn();

// Stub IntakeClient + capturePage from @intake/core (single mock factory).
vi.mock('@intake/core', async (orig) => {
  const actual = (await orig()) as Record<string, unknown>;
  function IntakeClient() {
    return { init: mockInit, turn: mockTurn, submit: mockSubmit };
  }
  const fakeCanvas = { width: 200, height: 100 } as unknown as HTMLCanvasElement;
  return {
    ...actual,
    IntakeClient,
    capturePage: vi.fn().mockResolvedValue(fakeCanvas),
  };
});

const CAPS_DISABLED = {
  session_id: 'sess-1',
  capabilities: { auth_modes: ['anonymous'], streaming: true },
};

const CAPS_ENABLED = {
  session_id: 'sess-1',
  capabilities: {
    auth_modes: ['anonymous'],
    streaming: true,
    attachments: {
      max_size_bytes: 5_242_880,
      max_total_bytes: 10_485_760,
      allowed_mime_types: ['image/png', 'image/jpeg', 'image/webp'],
    },
  },
};

// Canvas stubs (getContext/toDataURL/toBlob) are installed once in
// vue/vitest.setup.ts (Phase 6-iii cleanup); individual tests below override
// toDataURL when they need a deterministic return value.

beforeEach(() => {
  vi.clearAllMocks();
});

describe('IntakeWidget Attach button visibility', () => {
  it('Attach button is hidden when capabilities.attachments is absent', async () => {
    mockInit.mockResolvedValue(CAPS_DISABLED);
    const wrapper = mount(IntakeWidget, { props: { relayUrl: 'http://x' } });
    await flushPromises();
    await wrapper.find('[data-testid="launcher-button"]').trigger('click');
    expect(wrapper.find('[data-testid="attach-button"]').exists()).toBe(false);
  });

  it('Attach button is visible when capabilities.attachments is present', async () => {
    mockInit.mockResolvedValue(CAPS_ENABLED);
    const wrapper = mount(IntakeWidget, { props: { relayUrl: 'http://x' } });
    await flushPromises();
    await wrapper.find('[data-testid="launcher-button"]').trigger('click');
    expect(wrapper.find('[data-testid="attach-button"]').exists()).toBe(true);
  });
});

describe('IntakeWidget Attach → modal → strip → Submit flow', () => {
  it('clicking Attach opens the ScreenshotRedactor modal with the captured canvas', async () => {
    mockInit.mockResolvedValue(CAPS_ENABLED);
    const wrapper = mount(IntakeWidget, { props: { relayUrl: 'http://x' } });
    await flushPromises();
    await wrapper.find('[data-testid="launcher-button"]').trigger('click');
    await wrapper.find('[data-testid="attach-button"]').trigger('click');
    await flushPromises();
    expect(wrapper.find('[data-testid="redactor-modal"]').exists()).toBe(true);
  });

  it('saving the redactor pushes into the pending list, strip becomes visible', async () => {
    mockInit.mockResolvedValue(CAPS_ENABLED);
    const wrapper = mount(IntakeWidget, { props: { relayUrl: 'http://x' } });
    await flushPromises();
    await wrapper.find('[data-testid="launcher-button"]').trigger('click');
    await wrapper.find('[data-testid="attach-button"]').trigger('click');
    await flushPromises();

    // Stub toDataURL so Save emits a deterministic value.
    HTMLCanvasElement.prototype.toDataURL = vi
      .fn()
      .mockReturnValue('data:image/png;base64,AAAA') as unknown as typeof HTMLCanvasElement.prototype.toDataURL;

    await wrapper.find('[data-testid="redactor-save"]').trigger('click');
    await flushPromises();

    expect(wrapper.find('[data-testid="redactor-modal"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="attachment-strip"]').exists()).toBe(true);
    expect(wrapper.findAll('[data-testid="attachment-thumb"]')).toHaveLength(1);
  });

  it('Submit threads attachments through to client.submit', async () => {
    mockInit.mockResolvedValue(CAPS_ENABLED);
    mockSubmit.mockResolvedValue({
      external_id: 't-1',
      external_url: '',
      adapter_name: 'webhook',
      created_at: '2026-05-28T00:00:00Z',
    });
    const wrapper = mount(IntakeWidget, { props: { relayUrl: 'http://x' } });
    await flushPromises();
    await wrapper.find('[data-testid="launcher-button"]').trigger('click');

    // Get a message in so Submit is enabled.
    await wrapper.find('[data-testid="message-input"]').setValue('hi');
    mockTurn.mockResolvedValue({ input_tokens: 1, output_tokens: 1 });
    await wrapper.find('[data-testid="send-button"]').trigger('click');
    await flushPromises();

    await wrapper.find('[data-testid="attach-button"]').trigger('click');
    await flushPromises();
    HTMLCanvasElement.prototype.toDataURL = vi
      .fn()
      .mockReturnValue('data:image/png;base64,AAAA') as unknown as typeof HTMLCanvasElement.prototype.toDataURL;
    await wrapper.find('[data-testid="redactor-save"]').trigger('click');
    await flushPromises();

    await wrapper.find('[data-testid="submit-button"]').trigger('click');
    await flushPromises();

    expect(mockSubmit).toHaveBeenCalledOnce();
    const args = mockSubmit.mock.calls[0] as [unknown, unknown, unknown[]];
    expect(args[2]).toHaveLength(1);
    expect((args[2][0] as { mime_type: string }).mime_type).toBe('image/png');
  });
});
