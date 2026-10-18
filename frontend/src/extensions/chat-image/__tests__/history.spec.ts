import { beforeEach, describe, expect, it, vi } from 'vitest'

const get = vi.fn()
const post = vi.fn()
const del = vi.fn()

vi.mock('@/api/client', () => ({
  apiClient: { get, post, delete: del }
}))

describe('chat-image persistent history API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('maps database response fields into view models', async () => {
    get.mockResolvedValue({
      data: {
        id: 9,
        title: '已保存会话',
        message_count: 1,
        preview: '结果',
        created_at: '2026-09-13T01:00:00Z',
        updated_at: '2026-09-13T01:01:00Z',
        messages: [{
          id: 11,
          client_id: '41969fab-2898-4a39-9e2f-8f58ac6a6020',
          role: 'assistant',
          kind: 'images',
          content: '',
          model: 'gpt-image-2',
          created_at: '2026-09-13T01:01:00Z',
          resources: [{
            id: 12,
            type: 'image',
            url: 'https://cdn.example.com/image.png',
            display_name: 'image.png',
            mime_type: 'image/png',
            size_bytes: 1024,
            created_at: '2026-09-13T01:01:00Z'
          }]
        }]
      }
    })

    const { getConversation } = await import('../history')
    const conversation = await getConversation(9)

    expect(conversation.loaded).toBe(true)
    expect(conversation.messageCount).toBe(1)
    expect(conversation.messages[0].clientId).toBe('41969fab-2898-4a39-9e2f-8f58ac6a6020')
    expect(conversation.messages[0].resources[0]).toMatchObject({
      url: 'https://cdn.example.com/image.png',
      displayName: 'image.png',
      mimeType: 'image/png',
      sizeBytes: 1024
    })
  })

  it('sends only resource URLs and metadata when appending a message', async () => {
    post.mockResolvedValue({
      data: {
        message: {
          id: 11,
          client_id: '41969fab-2898-4a39-9e2f-8f58ac6a6020',
          role: 'assistant',
          kind: 'files',
          content: '',
          resources: [],
          created_at: '2026-09-13T01:01:00Z'
        },
        title: '文件',
        message_count: 1,
        updated_at: '2026-09-13T01:01:00Z'
      }
    })
    const { appendMessage } = await import('../history')

    await appendMessage(9, {
      clientId: '41969fab-2898-4a39-9e2f-8f58ac6a6020',
      role: 'assistant',
      kind: 'files',
      content: '',
      resources: [{
        type: 'file',
        url: '/files/report.pdf',
        displayName: 'report.pdf',
        mimeType: 'application/pdf',
        sizeBytes: 2048
      }]
    })

    expect(post).toHaveBeenCalledWith('/chat-image/conversations/9/messages', {
      client_id: '41969fab-2898-4a39-9e2f-8f58ac6a6020',
      role: 'assistant',
      kind: 'files',
      content: '',
      model: '',
      resources: [{
        type: 'file',
        url: '/files/report.pdf',
        display_name: 'report.pdf',
        mime_type: 'application/pdf',
        size_bytes: 2048
      }]
    })
  })
})
