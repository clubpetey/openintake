import { describe, it, expect } from 'vitest';
import { mount } from '@vue/test-utils';
import ConversationView from './ConversationView.vue';
import type { ChatMessage } from '@openintake/core';

describe('ConversationView', () => {
  it('renders nothing when messages is empty', () => {
    const wrapper = mount(ConversationView, {
      props: { messages: [], streaming: false },
    });
    expect(wrapper.findAll('[data-testid="message"]')).toHaveLength(0);
  });

  it('renders a user message with correct role class', () => {
    const messages: ChatMessage[] = [{ role: 'user', content: 'Hello!' }];
    const wrapper = mount(ConversationView, {
      props: { messages, streaming: false },
    });
    const msgs = wrapper.findAll('[data-testid="message"]');
    expect(msgs).toHaveLength(1);
    expect(msgs[0].classes()).toContain('message--user');
    expect(msgs[0].text()).toContain('Hello!');
  });

  it('renders an assistant message with correct role class', () => {
    const messages: ChatMessage[] = [{ role: 'assistant', content: 'How can I help?' }];
    const wrapper = mount(ConversationView, {
      props: { messages, streaming: false },
    });
    const msgs = wrapper.findAll('[data-testid="message"]');
    expect(msgs).toHaveLength(1);
    expect(msgs[0].classes()).toContain('message--assistant');
  });

  it('renders multiple messages in order', () => {
    const messages: ChatMessage[] = [
      { role: 'user', content: 'First' },
      { role: 'assistant', content: 'Second' },
      { role: 'user', content: 'Third' },
    ];
    const wrapper = mount(ConversationView, {
      props: { messages, streaming: false },
    });
    const msgs = wrapper.findAll('[data-testid="message"]');
    expect(msgs).toHaveLength(3);
    expect(msgs[0].text()).toContain('First');
    expect(msgs[1].text()).toContain('Second');
    expect(msgs[2].text()).toContain('Third');
  });

  it('shows streaming indicator when streaming=true', () => {
    const wrapper = mount(ConversationView, {
      props: { messages: [], streaming: true },
    });
    expect(wrapper.find('[data-testid="streaming-indicator"]').exists()).toBe(true);
  });

  it('hides streaming indicator when streaming=false', () => {
    const wrapper = mount(ConversationView, {
      props: { messages: [], streaming: false },
    });
    expect(wrapper.find('[data-testid="streaming-indicator"]').exists()).toBe(false);
  });
});
