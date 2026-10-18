import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ImageGalleryDialog from './ImageGalleryDialog.vue'

const mocks = vi.hoisted(() => ({
  listGalleryImages: vi.fn(),
  deleteGalleryImages: vi.fn(),
  downloadGalleryImages: vi.fn(),
  saveAs: vi.fn()
}))

vi.mock('./api', () => ({
  listGalleryImages: mocks.listGalleryImages,
  deleteGalleryImages: mocks.deleteGalleryImages,
  downloadGalleryImages: mocks.downloadGalleryImages
}))

vi.mock('file-saver', () => ({ saveAs: mocks.saveAs }))

const items = [
  {
    id: 17,
    url: 'https://cdn.example.test/one.png',
    displayName: 'one.png',
    mimeType: 'image/png',
    createdAt: '2026-09-13T01:00:00Z',
    messageId: 10,
    model: 'gpt-image-2.5',
    conversationId: 5,
    conversationName: '第一组'
  },
  {
    id: 18,
    url: 'https://cdn.example.test/two.png',
    displayName: 'two.png',
    mimeType: 'image/png',
    createdAt: '2026-09-13T01:01:00Z',
    messageId: 11,
    model: 'gpt-image-2.5',
    conversationId: 6,
    conversationName: '第二组'
  }
]

function mountDialog() {
  return mount(ImageGalleryDialog, {
    props: { show: true },
    global: {
      stubs: {
        Teleport: true,
        Icon: true,
        ImageLightbox: true
      }
    }
  })
}

describe('ImageGalleryDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.listGalleryImages.mockResolvedValue({
      items,
      total: items.length,
      page: 1,
      pageSize: 24,
      pages: 1
    })
    mocks.deleteGalleryImages.mockResolvedValue({
      deletedIds: [17],
      deletedUrls: [items[0].url],
      deleted: 1
    })
    mocks.downloadGalleryImages.mockResolvedValue({
      blob: new Blob(['image'], { type: 'image/png' }),
      filename: 'one.png'
    })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
  })

  it('shows all loaded records and switches between single and ZIP download actions', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    expect(wrapper.findAll('.gallery-item')).toHaveLength(2)
    const choices = wrapper.findAll<HTMLInputElement>('.image-checkbox input')

    await choices[0].setValue(true)
    expect(wrapper.text()).toContain('下载图片')
    await wrapper.findAll('.gallery-toolbar button')[0].trigger('click')
    await flushPromises()
    expect(mocks.downloadGalleryImages).toHaveBeenCalledWith([17], expect.any(AbortSignal))
    expect(mocks.saveAs).toHaveBeenCalledWith(expect.any(Blob), 'one.png')

    await choices[1].setValue(true)
    expect(wrapper.text()).toContain('打包下载 (2)')
    wrapper.unmount()
  })

  it('deletes selected records and reports deleted IDs and URLs to the parent', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    await wrapper.find<HTMLInputElement>('.image-checkbox input').setValue(true)
    const deleteButton = wrapper.findAll('.gallery-toolbar button')
      .find((button) => button.text().includes('删除'))
    expect(deleteButton).toBeDefined()
    await deleteButton!.trigger('click')
    await flushPromises()

    expect(mocks.deleteGalleryImages).toHaveBeenCalledWith([17])
    expect(wrapper.emitted('deleted')?.[0]).toEqual([{
      deletedIds: [17],
      deletedUrls: [items[0].url],
      deleted: 1
    }])
    wrapper.unmount()
  })
})
