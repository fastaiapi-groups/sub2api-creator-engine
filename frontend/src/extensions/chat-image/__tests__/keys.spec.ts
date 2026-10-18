import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ApiKey, Group } from '@/types'

const list = vi.fn()
const create = vi.fn()
const update = vi.fn()

vi.mock('@/api', () => ({
  keysAPI: { list, create, update },
  userGroupsAPI: { getAvailable: vi.fn() }
}))

function group(id: number, name: string): Group {
  return { id, name } as Group
}

function apiKey(overrides: Partial<ApiKey> = {}): ApiKey {
  return {
    id: 10,
    user_id: 1,
    key: 'sk-existing',
    name: 'Chat',
    group_id: 3,
    status: 'active',
    ...overrides
  } as ApiKey
}

describe('chat-image fixed API keys', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('matches configured groups case-insensitively', async () => {
    const { findGroup } = await import('../keys')

    expect(findGroup([group(3, 'GPT-Pro')], 'gpt-pro').id).toBe(3)
    expect(() => findGroup([], 'gpt-pro')).toThrow('当前账号无权使用“gpt-pro”分组')
  })

  it('reuses an active fixed-name key in the requested group', async () => {
    const { ensureKey } = await import('../keys')
    list.mockResolvedValue({ items: [apiKey()] })

    await expect(ensureKey(group(3, 'gpt-pro'), 'Chat'))
      .resolves.toBe('sk-existing')
    expect(create).not.toHaveBeenCalled()
    expect(update).not.toHaveBeenCalled()
  })

  it('creates a fixed-name key when one does not exist', async () => {
    const { ensureKey } = await import('../keys')
    list.mockResolvedValue({ items: [] })
    create.mockResolvedValue(apiKey({ key: 'sk-created' }))

    await expect(ensureKey(group(3, 'gpt-pro'), 'Chat'))
      .resolves.toBe('sk-created')
    expect(create).toHaveBeenCalledWith('Chat', 3)
  })

  it('reactivates a manually disabled fixed-name key', async () => {
    const { ensureKey } = await import('../keys')
    const inactive = apiKey({ status: 'inactive' })
    list.mockResolvedValue({ items: [inactive] })
    update.mockResolvedValue(apiKey())

    await expect(ensureKey(group(3, 'gpt-pro'), 'Chat'))
      .resolves.toBe('sk-existing')
    expect(update).toHaveBeenCalledWith(10, { status: 'active' })
  })

  it('renames a legacy key instead of creating a duplicate', async () => {
    const { ensureKey } = await import('../keys')
    const legacy = apiKey({ name: '__chat_image_chat__' })
    list
      .mockResolvedValueOnce({ items: [] })
      .mockResolvedValueOnce({ items: [legacy] })
    update.mockResolvedValue(apiKey())

    await expect(ensureKey(group(3, 'gpt-pro'), 'Chat', ['__chat_image_chat__']))
      .resolves.toBe('sk-existing')
    expect(update).toHaveBeenCalledWith(10, { name: 'Chat' })
    expect(create).not.toHaveBeenCalled()
  })
})
