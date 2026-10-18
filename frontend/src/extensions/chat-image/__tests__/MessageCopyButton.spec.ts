import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import MessageCopyButton from '../MessageCopyButton.vue'

describe('MessageCopyButton', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('copies the complete response and shows temporary feedback', async () => {
    vi.useFakeTimers()
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    Object.defineProperty(window, 'isSecureContext', { configurable: true, value: true })

    const wrapper = mount(MessageCopyButton, {
      props: { content: 'First line\nSecond line' },
      global: { stubs: { Icon: true } }
    })

    await wrapper.get('button').trigger('click')
    await Promise.resolve()

    expect(writeText).toHaveBeenCalledWith('First line\nSecond line')
    expect(wrapper.text()).toContain('已复制')

    vi.advanceTimersByTime(2000)
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('复制')
    expect(wrapper.text()).not.toContain('已复制')
  })
})
