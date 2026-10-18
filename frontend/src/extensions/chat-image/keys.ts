import { keysAPI, userGroupsAPI } from '@/api'
import type { ApiKey, Group } from '@/types'
import {
  CHAT_GROUP_NAME,
  CHAT_KEY_NAME,
  IMAGE_GROUP_NAME,
  IMAGE_KEY_NAME,
  LEGACY_CHAT_KEY_NAMES,
  LEGACY_IMAGE_KEY_NAMES
} from './config'
import type { FeatureKeys } from './types'

let keysPromise: Promise<FeatureKeys> | null = null

export function findGroup(groups: Group[], expectedName: string): Group {
  const normalized = expectedName.toLocaleLowerCase()
  const group = groups.find((item) => item.name.trim().toLocaleLowerCase() === normalized)
  if (!group) {
    throw new Error(`当前账号无权使用“${expectedName}”分组，请先为用户开放该分组`)
  }
  return group
}

export async function ensureKey(
  group: Group,
  name: string,
  legacyNames: readonly string[] = []
): Promise<string> {
  const result = await keysAPI.list(1, 100, { search: name })
  let existing = result.items.find(
    (item: ApiKey) => item.name === name && item.group_id === group.id
  )

  if (!existing) {
    for (const legacyName of legacyNames) {
      const legacyResult = await keysAPI.list(1, 100, { search: legacyName })
      existing = legacyResult.items.find(
        (item: ApiKey) => item.name === legacyName && item.group_id === group.id
      )
      if (existing) break
    }
  }

  if (existing) {
    const updates = {
      ...(existing.name !== name ? { name } : {}),
      ...(existing.status !== 'active' ? { status: 'active' as const } : {})
    }
    if (Object.keys(updates).length) {
      existing = await keysAPI.update(existing.id, updates)
    }
  }
  if (!existing) {
    existing = await keysAPI.create(name, group.id)
  }
  if (!existing.key) {
    throw new Error(`无法读取“${name}”的 API Key`)
  }
  return existing.key
}

async function prepareKeys(): Promise<FeatureKeys> {
  const groups = await userGroupsAPI.getAvailable()
  const chatGroup = findGroup(groups, CHAT_GROUP_NAME)
  const imageGroup = findGroup(groups, IMAGE_GROUP_NAME)
  const [chat, image] = await Promise.all([
    ensureKey(chatGroup, CHAT_KEY_NAME, LEGACY_CHAT_KEY_NAMES),
    ensureKey(imageGroup, IMAGE_KEY_NAME, LEGACY_IMAGE_KEY_NAMES)
  ])
  return { chat, image }
}

export function ensureFeatureKeys(): Promise<FeatureKeys> {
  if (!keysPromise) {
    keysPromise = prepareKeys().catch((error) => {
      keysPromise = null
      throw error
    })
  }
  return keysPromise
}
