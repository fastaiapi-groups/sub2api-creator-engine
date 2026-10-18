import { beforeEach, describe, expect, it, vi } from 'vitest'

const get = vi.fn()
const post = vi.fn()
const del = vi.fn()

vi.mock('@/api/client', () => ({
  apiClient: { get, post, delete: del }
}))

describe('chat-image gallery API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('maps paginated image records into frontend models', async () => {
    get.mockResolvedValue({
      data: {
        items: [{
          id: 17,
          url: 'https://cdn.example.test/poster.png',
          display_name: 'poster.png',
          mime_type: 'image/png',
          size_bytes: 4096,
          created_at: '2026-09-13T01:00:00Z',
          message_id: 11,
          model: 'gpt-image-2.5',
          conversation_id: 5,
          conversation_name: '海报设计'
        }],
        total: 25,
        page: 2,
        page_size: 24,
        pages: 2
      }
    })

    const { listGalleryImages } = await import('./api')
    const result = await listGalleryImages(2, 24)

    expect(get).toHaveBeenCalledWith('/chat-image/gallery/images', expect.objectContaining({
      params: { page: 2, page_size: 24 }
    }))
    expect(result).toMatchObject({ total: 25, page: 2, pageSize: 24, pages: 2 })
    expect(result.items[0]).toMatchObject({
      id: 17,
      displayName: 'poster.png',
      mimeType: 'image/png',
      sizeBytes: 4096,
      conversationName: '海报设计'
    })
  })

  it('maps deleted records and sends the selected IDs', async () => {
    del.mockResolvedValue({
      data: {
        deleted_ids: [17, 18],
        deleted_urls: ['https://cdn.example.test/poster.png'],
        deleted: 2
      }
    })

    const { deleteGalleryImages } = await import('./api')
    await expect(deleteGalleryImages([17])).resolves.toEqual({
      deletedIds: [17, 18],
      deletedUrls: ['https://cdn.example.test/poster.png'],
      deleted: 2
    })
    expect(del).toHaveBeenCalledWith('/chat-image/gallery/images', { data: { ids: [17] } })
  })

  it('uses the UTF-8 response filename for single files and archives', async () => {
    post.mockResolvedValue({
      data: new Blob(['image']),
      headers: {
        'content-disposition': "attachment; filename=\"poster.png\"; filename*=UTF-8''%E6%B5%B7%E6%8A%A5.png"
      }
    })

    const { downloadGalleryImages, filenameFromDisposition } = await import('./api')
    await expect(downloadGalleryImages([17])).resolves.toMatchObject({ filename: '海报.png' })
    expect(post).toHaveBeenCalledWith(
      '/chat-image/gallery/images/download',
      { ids: [17] },
      expect.objectContaining({ responseType: 'blob' })
    )
    expect(filenameFromDisposition('', 'images.zip')).toBe('images.zip')
  })
})
