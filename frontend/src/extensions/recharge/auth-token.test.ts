import { afterEach, beforeEach, expect, it, vi } from 'vitest'

beforeEach(() => {
  vi.resetModules()
  localStorage.clear()
  window.history.replaceState({}, '', '/fastaiui/recharge?token=embedded-token&order=RA-1')
})

afterEach(() => {
  localStorage.clear()
  window.history.replaceState({}, '', '/')
})

it('uses an embedded token when the iframe origin has no local session', async () => {
  const { getAuthToken } = await import('../../api/client')

  expect(getAuthToken()).toBe('embedded-token')
  expect(window.location.search).toBe('?order=RA-1')
})
