export type ClientAsset = {
  platform: string
  arch: string
  name: string
  url: string
  size_bytes: number
  sha256: string
}

export type ClientRelease = {
  id: number
  product_id: string
  version: string
  channel: string
  status: string
  notes: string
  assets: ClientAsset[]
  required_targets: string[]
  options?: { mandatory: boolean; runtime_sha256?: string }
}
