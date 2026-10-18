export type MessageRole = 'user' | 'assistant'
export type MessageKind = 'text' | 'images' | 'files' | 'error'

export interface ChatImageResource {
  id: number
  type: 'image' | 'file'
  url: string
  displayName?: string
  mimeType?: string
  sizeBytes?: number
  createdAt: string
}

export interface ChatImageMessage {
  id: number
  clientId: string
  role: MessageRole
  kind: MessageKind
  content: string
  model?: string
  resources: ChatImageResource[]
  /** Non-persistent base64 results returned by a channel are kept for this page session only. */
  transientImages?: string[]
  createdAt: string
}

export interface ChatImageConversation {
  id: number
  title: string
  messageCount: number
  preview: string
  messages: ChatImageMessage[]
  loaded: boolean
  createdAt: string
  updatedAt: string
}

export interface ChatImageListResult {
  items: ChatImageConversation[]
  total: number
  page: number
  pageSize: number
  pages: number
}

export interface ChatImageResourceInput {
  type: 'image' | 'file'
  url: string
  displayName?: string
  mimeType?: string
  sizeBytes?: number
}

export interface ChatImageMessageInput {
  clientId: string
  role: MessageRole
  kind: MessageKind
  content: string
  model?: string
  resources?: ChatImageResourceInput[]
}

export interface FeatureKeys {
  chat: string
  image: string
}

export interface ImageOptions {
  model: string
  quality: 'auto' | 'low' | 'medium' | 'high'
  count: 1 | 2 | 3 | 4
  size: string
}

export interface ImageTaskProgress {
  taskId: string
  stage: 'submitting' | 'processing' | 'fallback'
  label: string
}
