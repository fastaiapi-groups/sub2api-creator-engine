import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import MessageAvatar from '../MessageAvatar.vue'

describe('MessageAvatar', () => {
  it('uses the FastAiApi project logo for user messages', () => {
    const wrapper = mount(MessageAvatar, { props: { role: 'user' } })

    expect(wrapper.get('img').attributes('src')).toContain('fastapapi-logo.svg')
    expect(wrapper.attributes('aria-label')).toBe('用户头像')
  })

  it('uses the GPT logo for assistant messages', () => {
    const wrapper = mount(MessageAvatar, { props: { role: 'assistant' } })

    expect(wrapper.get('img').attributes('src')).toContain('gpt-logo.svg')
    expect(wrapper.attributes('aria-label')).toBe('GPT 助手')
  })
})
