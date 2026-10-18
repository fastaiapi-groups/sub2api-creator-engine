import { describe, expect, it } from 'vitest'
import {
  filterPrompts,
  promptCategory,
  promptSummary,
  validatePromptLibrary,
  type PromptLibraryItem,
  type PromptLibraryView
} from '../promptLibrary'

const item: PromptLibraryItem = {
  id: 'source:poster',
  source_id: 'source',
  source_name: 'Example Source',
  title: '商业海报',
  prompt: '生成一张现代商业海报',
  description: '营销设计模板',
  preview: 'https://example.test/poster.png',
  link: '',
  author: '',
  category: '工作',
  sub_category: '设计',
  tags: ['海报'],
  reference_image_urls: [],
  image_mode: 'generate',
  image_model: 'gpt-image-2',
  image_size: '1024x1024',
  image_count: 1
}

describe('prompt library', () => {
  it('filters across title, category, source and prompt text', () => {
    expect(filterPrompts([item], '营销', 'all', 'all')).toEqual([item])
    expect(filterPrompts([item], 'example', 'all', 'all')).toEqual([item])
    expect(filterPrompts([item], '海报', 'source', '工作 / 设计')).toEqual([item])
    expect(filterPrompts([item], '', 'other', 'all')).toEqual([])
  })

  it('formats prompt metadata and validates counts', () => {
    expect(promptCategory(item)).toBe('工作 / 设计')
    expect(promptSummary(item)).toBe('营销设计模板')
    const view = {
      schema_version: 1,
      version: 'test',
      prompt_count: 1,
      source_count: 1,
      enabled_source_count: 1,
      items: [item],
      sources: [{ id: 'source', name: 'Example Source', homepage: '', enabled: true, prompt_count: 1 }]
    } satisfies PromptLibraryView
    expect(validatePromptLibrary(view)).toBe(view)
    expect(() => validatePromptLibrary({ ...view, prompt_count: 2 })).toThrow('数量')
  })
})
