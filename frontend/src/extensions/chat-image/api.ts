import { buildGatewayUrl } from '@/api/client'
import type { ChatImageMessage, ChatImageResourceInput, ImageOptions, ImageTaskProgress } from './types'
import { normalizeImageURL } from './imageUrl'

export interface ChatCompletionResult {
  content: string
  resources: ChatImageResourceInput[]
}

class GatewayRequestError extends Error {
  constructor(public readonly status: number, message: string) {
    super(message)
    this.name = 'GatewayRequestError'
  }
}

async function gatewayResponse<T>(
  path: string,
  key: string,
  init: RequestInit = {}
): Promise<{ payload: T; response: Response }> {
  const response = await fetch(buildGatewayUrl(path), {
    ...init,
    headers: {
      Authorization: `Bearer ${key}`,
      Accept: 'application/json',
      ...(init.body instanceof FormData ? {} : { 'Content-Type': 'application/json' }),
      ...init.headers
    }
  })
  const raw = await response.text()
  let payload: any = null
  try {
    payload = raw ? JSON.parse(raw) : null
  } catch {
    payload = raw
  }
  if (!response.ok) {
    const message = payload?.error?.message || payload?.message || raw || response.statusText
    throw new GatewayRequestError(response.status, `${response.status}: ${String(message).slice(0, 300)}`)
  }
  return { payload: payload as T, response }
}

async function gatewayRequest<T>(path: string, key: string, init: RequestInit = {}): Promise<T> {
  return (await gatewayResponse<T>(path, key, init)).payload
}

export async function listModels(key: string): Promise<string[]> {
  const payload = await gatewayRequest<{ data?: Array<{ id?: string } | string> }>(
    '/v1/models',
    key
  )
  return parseModelIds(payload)
}

export function parseModelIds(payload: { data?: Array<{ id?: string } | string> }): string[] {
  return (payload.data || [])
    .map((item) => (typeof item === 'string' ? item : item.id || ''))
    .filter(Boolean)
    .sort((a, b) => a.localeCompare(b))
}

export async function sendChat(
  key: string,
  model: string,
  messages: ChatImageMessage[]
): Promise<ChatCompletionResult> {
  const payload = await gatewayRequest<any>('/v1/chat/completions', key, {
    method: 'POST',
    body: JSON.stringify({
      model,
      messages: messages
        .filter((message) => message.kind === 'text')
        .map((message) => ({ role: message.role, content: message.content })),
      stream: false
    })
  })
  return parseChatCompletion(payload)
}

export async function sendChatStream(
  key: string,
  model: string,
  messages: ChatImageMessage[],
  onDelta: (text: string) => void,
  signal?: AbortSignal
): Promise<ChatCompletionResult> {
  const response = await fetch(buildGatewayUrl('/v1/chat/completions'), {
    method: 'POST',
    signal,
    headers: {
      Authorization: `Bearer ${key}`,
      Accept: 'text/event-stream',
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      model,
      messages: messages.filter((message) => message.kind === 'text').map((message) => ({ role: message.role, content: message.content })),
      stream: true
    })
  })
  if (!response.ok || !response.body) {
    const raw = await response.text()
    let payload: any = null
    try { payload = raw ? JSON.parse(raw) : null } catch { payload = null }
    throw new GatewayRequestError(response.status, payload?.error?.message || payload?.message || raw || response.statusText)
  }

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let content = ''
  const resources: ChatImageResourceInput[] = []
  const consume = (line: string) => {
    const value = line.startsWith('data:') ? line.slice(5).trim() : ''
    if (!value || value === '[DONE]') return
    let payload: any
    try { payload = JSON.parse(value) } catch { return }
    const delta = payload?.choices?.[0]?.delta?.content
    const text = typeof delta === 'string' ? delta : Array.isArray(delta) ? delta.map((part: any) => part?.text || '').join('') : ''
    if (text) { content += text; onDelta(text) }
    const candidates = payload?.choices?.[0]?.delta?.attachments || payload?.files || []
    for (const item of Array.isArray(candidates) ? candidates : []) {
      const resource = parseStructuredResource(item)
      if (resource && !resources.some((existing) => existing.url === resource.url)) resources.push(resource)
    }
  }
  while (true) {
    const { value, done } = await reader.read()
    buffer += decoder.decode(value || new Uint8Array(), { stream: !done })
    const lines = buffer.split(/\r?\n/)
    buffer = lines.pop() || ''
    lines.forEach(consume)
    if (done) break
  }
  if (buffer) consume(buffer)
  if (!content && resources.length === 0) throw new Error('流式响应中没有文字或文件内容')
  return { content, resources }
}

export function parseChatCompletion(payload: any): ChatCompletionResult {
  const message = payload?.choices?.[0]?.message
  const rawContent = message?.content
  const content = typeof rawContent === 'string'
    ? rawContent
    : Array.isArray(rawContent)
      ? rawContent.map((part) => part?.text || part?.content || '').filter(Boolean).join('\n')
      : ''
  const candidates = [
    ...(Array.isArray(message?.attachments) ? message.attachments : []),
    ...(Array.isArray(message?.files) ? message.files : []),
    ...(Array.isArray(payload?.files) ? payload.files : []),
    ...(Array.isArray(rawContent) ? rawContent : [])
  ]
  const resources = candidates
    .map(parseStructuredResource)
    .filter((resource): resource is ChatImageResourceInput => Boolean(resource))
    .filter((resource, index, items) => items.findIndex((item) => item.url === resource.url) === index)

  if (!content && resources.length === 0) {
    throw new Error('模型返回成功，但响应中没有文字或文件内容')
  }
  return { content, resources }
}

function parseStructuredResource(item: any): ChatImageResourceInput | null {
  if (!item || typeof item !== 'object') return null
  const nestedURL = typeof item.image_url === 'string' ? item.image_url : item.image_url?.url
  const url = item.url || item.file_url || item.download_url || item.path || nestedURL
  if (typeof url !== 'string' || !url) return null
  const mimeType = String(item.mime_type || item.mimeType || '')
  const itemType = String(item.type || '')
  const type = mimeType.startsWith('image/') || itemType.includes('image') ? 'image' : 'file'
  const rawSize = item.size_bytes ?? item.size
  const sizeBytes = typeof rawSize === 'number' && Number.isSafeInteger(rawSize) && rawSize >= 0
    ? rawSize
    : undefined
  return {
    type,
    url,
    displayName: String(item.display_name || item.filename || item.name || ''),
    mimeType,
    sizeBytes
  }
}

export function parseImageSources(payload: any): string[] {
  const items = Array.isArray(payload?.data)
    ? payload.data
    : Array.isArray(payload?.output)
      ? payload.output
      : []
  return items
    .map((item: any) => {
      if (typeof item === 'string') return item
      if (item?.url) return item.url
      if (item?.b64_json) return `data:image/png;base64,${item.b64_json}`
      if (item?.result) return `data:image/png;base64,${item.result}`
      return ''
    })
    .filter(Boolean)
    .map(normalizeImageURL)
}

interface ImageTaskPayload {
  id?: string
  task_id?: string
  status?: 'processing' | 'completed' | 'failed'
  result?: any
  image_url?: string
  error?: { message?: string } | string
}

export interface ImageGenerationResult {
  images: string[]
  taskId: string
  asynchronous: boolean
}

function imageRequestBody(prompt: string, options: ImageOptions): Record<string, unknown> {
  return {
    model: options.model,
    prompt,
    n: options.count,
    quality: options.quality,
    response_format: 'url',
    ...(options.size === 'auto' ? {} : { size: options.size })
  }
}

function imageEditForm(prompt: string, options: ImageOptions, referenceImage: File): FormData {
  const form = new FormData()
  form.append('image', referenceImage)
  form.append('model', options.model)
  form.append('prompt', prompt)
  form.append('n', String(options.count))
  form.append('quality', options.quality)
  form.append('response_format', 'url')
  if (options.size !== 'auto') form.append('size', options.size)
  return form
}

function imageEditBody(prompt: string, options: ImageOptions, referenceURL: string): Record<string, unknown> {
  return {
    ...imageRequestBody(prompt, options),
    images: [{ image_url: normalizeImageURL(referenceURL) }]
  }
}

export function parseImageTask(payload: ImageTaskPayload): {
  id: string
  status: 'processing' | 'completed' | 'failed'
  images: string[]
  error: string
} {
  const id = String(payload?.task_id || payload?.id || '').trim()
  const status = payload?.status
  if (!id || !status || !['processing', 'completed', 'failed'].includes(status)) {
    throw new Error('图片任务响应格式不正确')
  }
  const images = parseImageSources(payload.result)
  const imageURL = normalizeImageURL(payload.image_url || '')
  if (imageURL && !images.includes(imageURL)) images.unshift(imageURL)
  const error = typeof payload.error === 'string' ? payload.error : payload.error?.message || ''
  return { id, status, images, error }
}

export async function generateImages(
  key: string,
  prompt: string,
  options: ImageOptions,
  referenceImage?: File | string | null,
  onProgress?: (progress: ImageTaskProgress) => void,
  signal?: AbortSignal
): Promise<ImageGenerationResult> {
  onProgress?.({ taskId: '', stage: 'submitting', label: '等待账号' })
  onProgress?.({ taskId: '', stage: 'processing', label: '图片渠道处理中' })

  let payload: any
  if (referenceImage) {
    const body = typeof referenceImage === 'string'
      ? JSON.stringify(imageEditBody(prompt, options, referenceImage))
      : imageEditForm(prompt, options, referenceImage)
    payload = await gatewayRequest('/v1/images/edits', key, {
      method: 'POST',
      body,
      signal
    })
  } else {
    payload = await gatewayRequest('/v1/images/generations', key, {
      method: 'POST',
      body: JSON.stringify(imageRequestBody(prompt, options)),
      signal
    })
  }

  const images = parseImageSources(payload)
  if (images.length === 0) {
    throw new Error('图片接口返回成功，但响应中没有可展示的图片')
  }
  return { images, taskId: '', asynchronous: false }
}
