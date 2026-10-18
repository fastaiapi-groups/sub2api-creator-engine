export interface GalleryImage {
  id: number
  url: string
  displayName: string
  mimeType: string
  sizeBytes?: number
  createdAt: string
  messageId: number
  model: string
  conversationId: number
  conversationName: string
}

export interface GalleryListResult {
  items: GalleryImage[]
  total: number
  page: number
  pageSize: number
  pages: number
}

export interface GalleryDeleteResult {
  deletedIds: number[]
  deletedUrls: string[]
  deleted: number
}

export interface GalleryDownloadResult {
  blob: Blob
  filename: string
}
