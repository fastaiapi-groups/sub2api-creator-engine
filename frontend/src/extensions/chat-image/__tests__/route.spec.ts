import { describe, expect, it } from 'vitest'
import { chatImageRoutes } from '../index'

describe('chat-image route', () => {
  it('registers an authenticated user route', () => {
    expect(chatImageRoutes).toHaveLength(1)
    expect(chatImageRoutes[0]).toMatchObject({
      path: '/chat-image',
      name: 'ChatImage',
      meta: { requiresAuth: true, requiresAdmin: false }
    })
  })
})
