import { describe, it, expect } from 'vitest';
import { mount } from '@vue/test-utils';
import AttachmentStrip from './AttachmentStrip.vue';
import type { PendingAttachment } from '@intake/core';

function mkAtt(sizeBytes: number, label?: string): PendingAttachment {
  return {
    type: 'screenshot',
    mimeType: 'image/png',
    dataUrl: 'data:image/png;base64,AAAA',
    label,
    sizeBytes,
  };
}

describe('AttachmentStrip', () => {
  it('is hidden when items is empty', () => {
    const wrapper = mount(AttachmentStrip, {
      props: { items: [], maxTotalBytes: 10_000_000 },
    });
    expect(wrapper.find('[data-testid="attachment-strip"]').exists()).toBe(false);
  });

  it('renders one thumbnail per item', () => {
    const wrapper = mount(AttachmentStrip, {
      props: {
        items: [mkAtt(1000), mkAtt(2000), mkAtt(3000)],
        maxTotalBytes: 10_000_000,
      },
    });
    const thumbs = wrapper.findAll('[data-testid="attachment-thumb"]');
    expect(thumbs).toHaveLength(3);
  });

  it('clicking remove on item N emits remove(N)', async () => {
    const wrapper = mount(AttachmentStrip, {
      props: {
        items: [mkAtt(1000), mkAtt(2000), mkAtt(3000)],
        maxTotalBytes: 10_000_000,
      },
    });
    const removes = wrapper.findAll('[data-testid="attachment-remove"]');
    await removes[1].trigger('click');
    expect(wrapper.emitted('remove')).toBeTruthy();
    expect(wrapper.emitted('remove')?.[0]?.[0]).toBe(1);
  });

  it('aggregate-size badge shows human-readable used/total (MB)', () => {
    const wrapper = mount(AttachmentStrip, {
      props: {
        items: [mkAtt(4_200_000)],
        maxTotalBytes: 10_485_760,
      },
    });
    const badge = wrapper.find('[data-testid="attachment-aggregate"]');
    expect(badge.exists()).toBe(true);
    // "4.2 MB / 10.0 MB" or similar; assert both numbers + "MB" appear.
    const text = badge.text();
    expect(text).toMatch(/4\.\d\s*MB/);
    expect(text).toMatch(/10(\.\d)?\s*MB/);
  });

  it('aggregate-size badge handles tiny payloads in KB', () => {
    const wrapper = mount(AttachmentStrip, {
      props: { items: [mkAtt(2_000)], maxTotalBytes: 10_000 },
    });
    const text = wrapper.find('[data-testid="attachment-aggregate"]').text();
    expect(text).toMatch(/2\.\d\s*KB/);
    expect(text).toMatch(/9\.\d\s*KB/);
  });
});
