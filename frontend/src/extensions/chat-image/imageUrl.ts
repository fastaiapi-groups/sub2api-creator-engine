const HTTPS_IMAGE_HOSTS = new Set([
  'chatgpt.fastaiapi.cloud'
])

export function normalizeImageURL(value: string): string {
  const source = String(value || '').trim()
  if (!source || source.startsWith('data:image/') || (source.startsWith('/') && !source.startsWith('//'))) {
    return source
  }
  try {
    const parsed = new URL(source)
    if (parsed.protocol === 'http:' && HTTPS_IMAGE_HOSTS.has(parsed.hostname.toLocaleLowerCase())) {
      parsed.protocol = 'https:'
      return parsed.toString()
    }
  } catch {
    return source
  }
  return source
}

