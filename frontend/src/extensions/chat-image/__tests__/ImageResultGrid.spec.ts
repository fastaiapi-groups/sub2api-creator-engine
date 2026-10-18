import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import ImageResultGrid from '../ImageResultGrid.vue'

describe('ImageResultGrid', () => {
  it.each([1, 2, 3, 4])('renders %i pending image slots', (count) => {
    const wrapper = mount(ImageResultGrid, {
      props: { sources: [], pendingCount: count }
    })

    expect(wrapper.findAll('.result-item')).toHaveLength(count)
    expect(wrapper.findAll('.pending-media')).toHaveLength(count)
  })

  it('renders all returned images and emits the selected preview index', async () => {
    const sources = [1, 2, 3, 4].map((index) => (
      `http://chatgpt.fastaiapi.cloud/images/result-${index}.png`
    ))
    const wrapper = mount(ImageResultGrid, { props: { sources } })

    expect(wrapper.findAll('img')).toHaveLength(4)
    expect(wrapper.findAll('img').map((image) => image.attributes('src'))).toEqual(
      [1, 2, 3, 4].map((index) => `https://chatgpt.fastaiapi.cloud/images/result-${index}.png`)
    )

    await wrapper.findAll('.result-media')[2].trigger('click')
    expect(wrapper.emitted('preview')).toEqual([[2]])

    await wrapper.findAll('[title="引用到输入框"]')[1].trigger('click')
    expect(wrapper.emitted('reference')).toEqual([[1]])
    expect(wrapper.text()).not.toContain('再次编辑')
  })
})
