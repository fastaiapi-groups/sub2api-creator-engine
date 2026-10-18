import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  generateImages,
  parseChatCompletion,
  parseImageSources,
  parseImageTask,
  parseModelIds
} from '../api'

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('chat-image API response parsing', () => {
  it('parses and sorts both model response shapes', () => {
    expect(parseModelIds({ data: [{ id: 'gpt-image-2' }, 'gpt-5.5', {}, { id: '' }] }))
      .toEqual(['gpt-5.5', 'gpt-image-2'])
  })

  it('parses image URLs and base64 payloads', () => {
    expect(parseImageSources({
      data: [
        { url: 'https://example.test/image.png' },
        { b64_json: 'AAAA' },
        { result: 'BBBB' }
      ]
    })).toEqual([
      'https://example.test/image.png',
      'data:image/png;base64,AAAA',
      'data:image/png;base64,BBBB'
    ])
  })

  it('keeps every image result and upgrades the supported channel host to HTTPS', () => {
    expect(parseImageSources({
      data: [1, 2, 3, 4].map((index) => ({
        url: `http://chatgpt.fastaiapi.cloud/images/result-${index}.png`
      }))
    })).toEqual([1, 2, 3, 4].map((index) => (
      `https://chatgpt.fastaiapi.cloud/images/result-${index}.png`
    )))
  })

  it('parses explicit file attachments without guessing URLs from text', () => {
    expect(parseChatCompletion({
      choices: [{
        message: {
          content: '报告在 https://example.test/page 中说明。',
          attachments: [{
            file_url: 'https://cdn.example.test/report.pdf',
            filename: 'report.pdf',
            mime_type: 'application/pdf',
            size_bytes: 2048
          }]
        }
      }]
    })).toEqual({
      content: '报告在 https://example.test/page 中说明。',
      resources: [{
        type: 'file',
        url: 'https://cdn.example.test/report.pdf',
        displayName: 'report.pdf',
        mimeType: 'application/pdf',
        sizeBytes: 2048
      }]
    })
  })

  it('parses asynchronous image task results and errors', () => {
    expect(parseImageTask({
      task_id: 'imgtask_123',
      status: 'completed',
      result: { data: [{ url: 'https://example.test/result.png' }] }
    })).toEqual({
      id: 'imgtask_123',
      status: 'completed',
      images: ['https://example.test/result.png'],
      error: ''
    })

    expect(parseImageTask({
      id: 'imgtask_failed',
      status: 'failed',
      error: { message: 'upstream unavailable' }
    }).error).toBe('upstream unavailable')

    expect(parseImageTask({
      task_id: 'imgtask_direct_url',
      status: 'completed',
      image_url: 'http://chatgpt.fastaiapi.cloud/images/direct.png'
    }).images).toEqual([
      'https://chatgpt.fastaiapi.cloud/images/direct.png'
    ])
  })

  it('requests synchronous URL image generation', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      data: [{ url: 'https://example.test/result.png' }]
    }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' }
    }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(generateImages('sk-test', 'draw a test', {
      model: 'gpt-image-2',
      quality: 'auto',
      count: 2,
      size: '2560x1440'
    })).resolves.toEqual({
      images: ['https://example.test/result.png'],
      taskId: '',
      asynchronous: false
    })

    const [url, init] = fetchMock.mock.calls[0]
    expect(String(url)).toContain('/v1/images/generations')
    expect(JSON.parse(String(init.body))).toEqual({
      model: 'gpt-image-2',
      prompt: 'draw a test',
      n: 2,
      quality: 'auto',
      response_format: 'url',
      size: '2560x1440'
    })
  })

  it('sends a generated URL directly to the synchronous edit endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      data: [{ url: 'https://example.test/edited.png' }]
    }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' }
    }))
    vi.stubGlobal('fetch', fetchMock)

    await generateImages('sk-test', 'make it blue', {
      model: 'gpt-image-2.5',
      quality: 'high',
      count: 1,
      size: '1024x1024'
    }, 'http://chatgpt.fastaiapi.cloud/images/source.png')

    const [url, init] = fetchMock.mock.calls[0]
    expect(String(url)).toContain('/v1/images/edits')
    expect(JSON.parse(String(init.body))).toMatchObject({
      response_format: 'url',
      images: [{ image_url: 'https://chatgpt.fastaiapi.cloud/images/source.png' }]
    })
  })
})
