import { apiClient } from '@/api/client'

export interface PromptLibraryItem {
  id: string
  source_id: string
  source_name: string
  title: string
  prompt: string
  description: string
  preview: string
  link: string
  author: string
  category: string
  sub_category: string
  tags: string[]
  reference_image_urls: string[]
  image_mode: '' | 'generate' | 'edit'
  image_model: string
  image_size: string
  image_count: number | null
}

export interface PromptLibrarySource {
  id: string
  name: string
  homepage: string
  enabled: boolean
  prompt_count: number
}

export interface PromptLibraryView {
  schema_version: 1
  version: string
  prompt_count: number
  source_count: number
  enabled_source_count: number
  items: PromptLibraryItem[]
  sources: PromptLibrarySource[]
}

let cachedLibrary: PromptLibraryView | null = null
let currentRequest: Promise<PromptLibraryView> | null = null

export async function loadPromptLibrary(force = false): Promise<PromptLibraryView> {
  if (!force && cachedLibrary) return cachedLibrary
  if (!force && currentRequest) return currentRequest
  currentRequest = apiClient.get<PromptLibraryView>('/chat-image/prompts')
    .then(({ data }) => validatePromptLibrary(data))
    .then((library) => {
      cachedLibrary = library
      return library
    })
    .finally(() => {
      currentRequest = null
    })
  return currentRequest
}

export function validatePromptLibrary(value: PromptLibraryView): PromptLibraryView {
  if (!value || value.schema_version !== 1 || !Array.isArray(value.items) || !Array.isArray(value.sources)) {
    throw new Error('提示词库响应格式不正确')
  }
  if (value.prompt_count !== value.items.length || value.source_count !== value.sources.length) {
    throw new Error('提示词库数量与内容不一致')
  }
  if (!value.items.every((item) => item.id && item.title && item.prompt)) {
    throw new Error('提示词库包含无效内容')
  }
  return value
}

export function promptCategory(item: PromptLibraryItem): string {
  return [item.category, item.sub_category].filter(Boolean).join(' / ')
}

export function promptSummary(item: PromptLibraryItem): string {
  return cleanDisplayText(item.description || item.prompt || promptCategory(item) || item.source_name)
}

export function cleanDisplayText(value: string): string {
  return String(value || '')
    .replace(/&nbsp;/gi, ' ')
    .replace(/&amp;/gi, '&')
    .replace(/&lt;/gi, '<')
    .replace(/&gt;/gi, '>')
    .replace(/&quot;/gi, '"')
    .replace(/&#39;|&apos;/gi, "'")
    .replace(/<\s*br\s*\/?>/gi, ' ')
    .replace(/<\/(p|div|li|h[1-6])>/gi, ' ')
    .replace(/<[^>]+>/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

export function filterPrompts(
  items: PromptLibraryItem[],
  keyword: string,
  sourceId: string,
  category: string
): PromptLibraryItem[] {
  const needle = keyword.trim().toLocaleLowerCase()
  return items.filter((item) => {
    if (sourceId !== 'all' && item.source_id !== sourceId) return false
    if (category !== 'all' && promptCategory(item) !== category) return false
    if (!needle) return true
    return [
      item.title,
      item.description,
      item.prompt,
      item.source_name,
      item.category,
      item.sub_category,
      ...item.tags
    ].join('\n').toLocaleLowerCase().includes(needle)
  })
}
