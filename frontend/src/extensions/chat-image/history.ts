import { apiClient } from '@/api/client'
import type {
  ChatImageConversation,
  ChatImageListResult,
  ChatImageMessage,
  ChatImageMessageInput,
  ChatImageResource
} from './types'

interface ResourceDTO {
  id: number
  type: 'image' | 'file'
  url: string
  display_name?: string
  mime_type?: string
  size_bytes?: number
  created_at: string
}

interface MessageDTO {
  id: number
  client_id: string
  role: ChatImageMessage['role']
  kind: ChatImageMessage['kind']
  content: string
  model?: string
  resources?: ResourceDTO[]
  created_at: string
}

interface ConversationDTO {
  id: number
  title: string
  message_count?: number
  preview?: string
  messages?: MessageDTO[]
  created_at: string
  updated_at: string
}

interface ListDTO {
  items?: ConversationDTO[]
  total: number
  page: number
  page_size: number
  pages: number
}

interface AppendResultDTO {
  message: MessageDTO
  title: string
  message_count: number
  updated_at: string
}

export interface AppendMessageResult {
  message: ChatImageMessage
  title: string
  messageCount: number
  updatedAt: string
}

function mapResource(resource: ResourceDTO): ChatImageResource {
  return {
    id: resource.id,
    type: resource.type,
    url: resource.url,
    displayName: resource.display_name,
    mimeType: resource.mime_type,
    sizeBytes: resource.size_bytes,
    createdAt: resource.created_at
  }
}

function mapMessage(message: MessageDTO): ChatImageMessage {
  return {
    id: message.id,
    clientId: message.client_id,
    role: message.role,
    kind: message.kind,
    content: message.content,
    model: message.model,
    resources: (message.resources || []).map(mapResource),
    createdAt: message.created_at
  }
}

function mapConversation(conversation: ConversationDTO): ChatImageConversation {
  const messages = (conversation.messages || []).map(mapMessage)
  return {
    id: conversation.id,
    title: conversation.title,
    messageCount: conversation.message_count ?? messages.length,
    preview: conversation.preview || '',
    messages,
    loaded: Array.isArray(conversation.messages),
    createdAt: conversation.created_at,
    updatedAt: conversation.updated_at
  }
}

export async function listConversations(
  page = 1,
  pageSize = 30,
  search = '',
  signal?: AbortSignal
): Promise<ChatImageListResult> {
  const { data } = await apiClient.get<ListDTO>('/chat-image/conversations', {
    params: { page, page_size: pageSize, ...(search ? { search } : {}) },
    signal
  })
  return {
    items: (data.items || []).map(mapConversation),
    total: data.total,
    page: data.page,
    pageSize: data.page_size,
    pages: data.pages
  }
}

export async function getConversation(id: number): Promise<ChatImageConversation> {
  const { data } = await apiClient.get<ConversationDTO>(`/chat-image/conversations/${id}`)
  return mapConversation(data)
}

export async function createConversation(title = ''): Promise<ChatImageConversation> {
  const { data } = await apiClient.post<ConversationDTO>('/chat-image/conversations', { title })
  return mapConversation(data)
}

export async function deleteConversation(id: number): Promise<void> {
  await apiClient.delete(`/chat-image/conversations/${id}`)
}

export async function clearConversation(id: number): Promise<ChatImageConversation> {
  const { data } = await apiClient.delete<ConversationDTO>(`/chat-image/conversations/${id}/messages`)
  return mapConversation(data)
}

export async function appendMessage(
  conversationId: number,
  input: ChatImageMessageInput
): Promise<AppendMessageResult> {
  const { data } = await apiClient.post<AppendResultDTO>(
    `/chat-image/conversations/${conversationId}/messages`,
    {
      client_id: input.clientId,
      role: input.role,
      kind: input.kind,
      content: input.content,
      model: input.model || '',
      resources: (input.resources || []).map((resource) => ({
        type: resource.type,
        url: resource.url,
        display_name: resource.displayName || '',
        mime_type: resource.mimeType || '',
        size_bytes: resource.sizeBytes
      }))
    }
  )
  return {
    message: mapMessage(data.message),
    title: data.title,
    messageCount: data.message_count,
    updatedAt: data.updated_at
  }
}
