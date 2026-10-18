import { apiClient } from '@/api/client'
import type {
  GalleryDeleteResult,
  GalleryDownloadResult,
  GalleryImage,
  GalleryListResult
} from './types'

interface GalleryImageDTO {
  id: number
  url: string
  display_name?: string
  mime_type?: string
  size_bytes?: number
  created_at: string
  message_id: number
  model?: string
  conversation_id: number
  conversation_name: string
}

interface GalleryListDTO {
  items?: GalleryImageDTO[]
  total: number
  page: number
  page_size: number
  pages: number
}

interface GalleryDeleteDTO {
  deleted_ids?: number[]
  deleted_urls?: string[]
  deleted: number
}

function mapImage(item: GalleryImageDTO): GalleryImage {
  return {
    id: item.id,
    url: item.url,
    displayName: item.display_name || '',
    mimeType: item.mime_type || '',
    sizeBytes: item.size_bytes,
    createdAt: item.created_at,
    messageId: item.message_id,
    model: item.model || '',
    conversationId: item.conversation_id,
    conversationName: item.conversation_name
  }
}

export async function listGalleryImages(
  page = 1,
  pageSize = 24,
  signal?: AbortSignal
): Promise<GalleryListResult> {
  const { data } = await apiClient.get<GalleryListDTO>('/chat-image/gallery/images', {
    params: { page, page_size: pageSize },
    signal
  })
  return {
    items: (data.items || []).map(mapImage),
    total: data.total,
    page: data.page,
    pageSize: data.page_size,
    pages: data.pages
  }
}

export async function deleteGalleryImages(ids: number[]): Promise<GalleryDeleteResult> {
  const { data } = await apiClient.delete<GalleryDeleteDTO>('/chat-image/gallery/images', {
    data: { ids }
  })
  return {
    deletedIds: data.deleted_ids || [],
    deletedUrls: data.deleted_urls || [],
    deleted: data.deleted
  }
}

export async function downloadGalleryImages(
  ids: number[],
  signal?: AbortSignal
): Promise<GalleryDownloadResult> {
  try {
    const response = await apiClient.post<Blob>(
      '/chat-image/gallery/images/download',
      { ids },
      { responseType: 'blob', signal, timeout: 10 * 60 * 1000 }
    )
    return {
      blob: response.data,
      filename: filenameFromDisposition(
        String(response.headers['content-disposition'] || ''),
        ids.length === 1 ? 'image.png' : 'images.zip'
      )
    }
  } catch (error: any) {
    const body = error?.response?.data
    if (body instanceof Blob && body.type.includes('json')) {
      try {
        const payload = JSON.parse(await body.text())
        throw new Error(payload?.message || '图片下载失败')
      } catch (parseError) {
        if (parseError instanceof SyntaxError) throw error
        throw parseError
      }
    }
    throw error
  }
}

export function filenameFromDisposition(value: string, fallback: string): string {
  const encoded = /filename\*=UTF-8''([^;]+)/i.exec(value)?.[1]
  if (encoded) {
    try {
      return decodeURIComponent(encoded)
    } catch {
      return fallback
    }
  }
  return /filename="([^"]+)"/i.exec(value)?.[1] || fallback
}
