import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ChatImageView from '../ChatImageView.vue'
import type { ChatImageMessageInput } from '../types'

const mocks = vi.hoisted(() => ({
  appendMessage: vi.fn(),
  generateImages: vi.fn(),
  listModels: vi.fn(),
  sendChat: vi.fn()
}))

vi.mock('../api', () => ({
  generateImages: mocks.generateImages,
  listModels: mocks.listModels,
  sendChat: mocks.sendChat
}))

vi.mock('../keys', () => ({
  ensureFeatureKeys: vi.fn().mockResolvedValue({ chat: 'chat-key', image: 'image-key' })
}))

vi.mock('../history', () => ({
  appendMessage: mocks.appendMessage,
  clearConversation: vi.fn(),
  createConversation: vi.fn(),
  deleteConversation: vi.fn(),
  getConversation: vi.fn(),
  listConversations: vi.fn().mockResolvedValue({
    items: [{
      id: 1,
      title: 'Test',
      messageCount: 0,
      preview: '',
      messages: [],
      loaded: true,
      createdAt: '2026-09-13T00:00:00Z',
      updatedAt: '2026-09-13T00:00:00Z'
    }],
    total: 1,
    page: 1,
    pageSize: 30,
    pages: 1
  })
}))

describe('ChatImageView composer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    Object.defineProperty(URL, 'createObjectURL', {
      configurable: true,
      value: vi.fn().mockReturnValue('blob:reference-preview')
    })
    Object.defineProperty(URL, 'revokeObjectURL', {
      configurable: true,
      value: vi.fn()
    })
    mocks.listModels.mockResolvedValue(['gpt-pro', 'gpt-image-2.5'])
    mocks.appendMessage.mockImplementation(async (_conversationId: number, input: ChatImageMessageInput) => ({
      message: {
        id: 1,
        clientId: input.clientId,
        role: input.role,
        kind: input.kind,
        content: input.content,
        model: input.model,
        resources: [],
        createdAt: '2026-09-13T00:00:00Z'
      },
      title: 'Test',
      messageCount: 1,
      updatedAt: '2026-09-13T00:00:00Z'
    }))
    HTMLElement.prototype.scrollTo = vi.fn()
  })

  it('clears the prompt and local reference before image generation finishes', async () => {
    let resolveGeneration!: (value: { images: string[]; taskId: string; asynchronous: boolean }) => void
    mocks.generateImages.mockImplementation(() => new Promise((resolve) => {
      resolveGeneration = resolve
    }))

    const wrapper = mount(ChatImageView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
          ImageLightbox: true,
          ImageResultGrid: true,
          PromptLibraryModal: true
        }
      }
    })
    await flushPromises()

    await wrapper.findAll('.mode-switch button')[1].trigger('click')
    const file = new File(['reference'], 'reference.png', { type: 'image/png' })
    const fileInput = wrapper.find<HTMLInputElement>('input[type="file"]')
    Object.defineProperty(fileInput.element, 'files', { configurable: true, value: [file] })
    await fileInput.trigger('change')
    await wrapper.find<HTMLTextAreaElement>('textarea').setValue('Edit this image')

    expect(wrapper.find('.reference-chip').exists()).toBe(true)
    await wrapper.find('.send-button').trigger('click')

    expect(wrapper.find<HTMLTextAreaElement>('textarea').element.value).toBe('')
    expect(wrapper.find('.reference-chip').exists()).toBe(false)
    await vi.waitFor(() => expect(mocks.generateImages).toHaveBeenCalledOnce())
    expect(mocks.generateImages.mock.calls[0][3]).toBe(file)

    resolveGeneration({
      images: ['https://example.com/result.png'],
      taskId: '',
      asynchronous: false
    })
    await flushPromises()
    wrapper.unmount()
  })
})
