import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import ImageLightbox from '../ImageLightbox.vue'

afterEach(() => {
  document.body.innerHTML = ''
})

describe('ImageLightbox', () => {
  it('shows the HTTPS original and navigates through multiple images', async () => {
    const wrapper = mount(ImageLightbox, {
      attachTo: document.body,
      props: {
        sources: [
          'http://chatgpt.fastaiapi.cloud/images/first.png',
          'http://chatgpt.fastaiapi.cloud/images/second.png'
        ],
        index: 0
      }
    })

    expect(document.body.querySelector('img')?.getAttribute('src'))
      .toBe('https://chatgpt.fastaiapi.cloud/images/first.png')

    document.body.querySelector<HTMLButtonElement>('.lightbox-nav.next')?.click()
    expect(wrapper.emitted('update:index')).toEqual([[1]])
    wrapper.unmount()
  })

  it('closes from the backdrop and Escape key', async () => {
    const wrapper = mount(ImageLightbox, {
      attachTo: document.body,
      props: { sources: ['https://example.test/original.png'], index: 0 }
    })

    document.body.querySelector<HTMLElement>('.image-lightbox')?.click()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))

    expect(wrapper.emitted('close')).toHaveLength(2)
    wrapper.unmount()
  })
})
