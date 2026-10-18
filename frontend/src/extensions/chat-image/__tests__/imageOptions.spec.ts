import { describe, expect, it } from 'vitest'
import {
  imageSizeFor,
  imageSizeSummary,
  isResolutionAvailable,
  selectionForImageSize
} from '../imageOptions'

describe('image options', () => {
  it('maps ratio and resolution to supported dimensions', () => {
    expect(imageSizeFor('1:1', '1K')).toBe('1024x1024')
    expect(imageSizeFor('16:9', '4K')).toBe('3840x2160')
    expect(imageSizeFor('2:3', '4K')).toBe('auto')
    expect(isResolutionAvailable('2:3', '4K')).toBe(false)
  })

  it('restores and formats a size selection', () => {
    expect(selectionForImageSize('1440x2560')).toEqual({ ratio: '9:16', resolution: '2K' })
    expect(imageSizeSummary('1440x2560')).toBe('9:16 · 2K · 1440x2560')
  })
})
