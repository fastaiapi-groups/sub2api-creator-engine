export type ImageRatio = 'auto' | '1:1' | '2:3' | '3:2' | '3:4' | '4:3' | '9:16' | '16:9'
export type ImageResolution = 'auto' | '1K' | '2K' | '4K'

interface ImageSizePreset {
  ratio: Exclude<ImageRatio, 'auto'>
  resolution: Exclude<ImageResolution, 'auto'>
  size: string
}

export const IMAGE_RATIOS: Array<{ value: ImageRatio; label: string }> = [
  { value: 'auto', label: '自动' },
  { value: '1:1', label: '1:1' },
  { value: '2:3', label: '2:3' },
  { value: '3:2', label: '3:2' },
  { value: '3:4', label: '3:4' },
  { value: '4:3', label: '4:3' },
  { value: '9:16', label: '9:16' },
  { value: '16:9', label: '16:9' }
]

export const IMAGE_RESOLUTIONS: Array<{ value: ImageResolution; label: string }> = [
  { value: 'auto', label: '自动' },
  { value: '1K', label: '1K' },
  { value: '2K', label: '2K' },
  { value: '4K', label: '4K' }
]

const PRESETS: ImageSizePreset[] = [
  { ratio: '1:1', resolution: '1K', size: '1024x1024' },
  { ratio: '2:3', resolution: '1K', size: '1024x1536' },
  { ratio: '3:2', resolution: '1K', size: '1536x1024' },
  { ratio: '3:4', resolution: '1K', size: '1024x1365' },
  { ratio: '4:3', resolution: '1K', size: '1365x1024' },
  { ratio: '9:16', resolution: '1K', size: '1088x1920' },
  { ratio: '16:9', resolution: '1K', size: '1920x1088' },
  { ratio: '1:1', resolution: '2K', size: '2048x2048' },
  { ratio: '9:16', resolution: '2K', size: '1440x2560' },
  { ratio: '16:9', resolution: '2K', size: '2560x1440' },
  { ratio: '9:16', resolution: '4K', size: '2160x3840' },
  { ratio: '16:9', resolution: '4K', size: '3840x2160' }
]

export function imageSizeFor(ratio: ImageRatio, resolution: ImageResolution): string {
  if (ratio === 'auto' || resolution === 'auto') return 'auto'
  return PRESETS.find((preset) => preset.ratio === ratio && preset.resolution === resolution)?.size || 'auto'
}

export function selectionForImageSize(size: string): { ratio: ImageRatio; resolution: ImageResolution } {
  if (!size || size === 'auto') return { ratio: 'auto', resolution: 'auto' }
  const preset = PRESETS.find((item) => item.size === size)
  return preset ? { ratio: preset.ratio, resolution: preset.resolution } : { ratio: 'auto', resolution: 'auto' }
}

export function isResolutionAvailable(ratio: ImageRatio, resolution: ImageResolution): boolean {
  if (resolution === 'auto' || ratio === 'auto') return true
  return PRESETS.some((preset) => preset.ratio === ratio && preset.resolution === resolution)
}

export function imageSizeSummary(size: string): string {
  if (!size || size === 'auto') return '自动'
  const preset = PRESETS.find((item) => item.size === size)
  return preset ? `${preset.ratio} · ${preset.resolution} · ${preset.size}` : size
}
