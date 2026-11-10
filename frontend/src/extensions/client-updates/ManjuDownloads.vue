<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue'
import QRCode from 'qrcode'
import type { ClientAsset, ClientRelease } from './types'
import './manju-downloads.css'
import androidRelease from './android-release.json'

const props = defineProps<{
  current: ClientRelease | null
  downloads: ClientRelease | null
  loading: boolean
  error: string
  notice: string
}>()
defineEmits<{ copy: [hash: string]; dismiss: [] }>()
const previousTitle = document.title
const androidQrCode = ref('')
onMounted(() => {
  document.title = '笑笑漫剧下载 · FastAiApi'
  if (androidRelease.published) void QRCode.toString(androidRelease.url, {
    type: 'svg', width: 192, margin: 4, errorCorrectionLevel: 'M',
    color: { dark: '#193b2d', light: '#ffffff' },
  }).then(svg => { androidQrCode.value = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}` }).catch(() => {})
})
onUnmounted(() => { document.title = previousTitle })

const installers = computed(() => props.downloads?.assets.filter(asset => asset.platform !== 'renderer') || [])
// Published notes are signed. Format their display without rewriting the release.
const releaseNotes = computed(() => {
  const notes = props.current?.notes.trim() || '本版本包含体验优化与问题修复。'
  return notes.replace(/\r\n?/g, '\n').split(/\n+/)
    .flatMap(line => line.split(/(?<=[。！？])\s*(?=\S)/u))
    .map(line => line.trim()).filter(Boolean)
})
const architecture = (asset: ClientAsset) => asset.platform === 'windows'
  ? '适用于 Windows 64 位'
  : asset.arch === 'arm64' ? '适用于 Mac · Apple 芯片'
    : asset.arch === 'x64' ? '适用于 Mac · Intel 芯片' : '适用于 Mac · Apple 芯片与 Intel'
const fileSize = (bytes: number) => `${(bytes / 1024 / 1024).toFixed(2)} MB`

function alternateDownload(value: string): string {
  try {
    const url = new URL(value)
    const prefix = '/fastaiapi/manju-updates/downloads/manju/'
    if (url.origin !== 'https://fastaiapi.cloud' || !url.pathname.startsWith(prefix)) return ''
    return `http://8.137.194.180:90/hub/updates/downloads/manju/${url.pathname.slice(prefix.length)}${url.search}`
  } catch {
    return ''
  }
}
</script>

<template>
  <div class="manju-downloads">
    <a class="md-skip-link" href="#downloads">跳到客户端下载</a>
    <header class="md-header">
      <a class="md-brand" href="/fastaiui/downloads/manju" aria-label="FastAiApi 笑笑漫剧下载首页">
        <span class="md-brand-mark" aria-hidden="true">F<span></span></span>
        <span>FastAiApi <small>笑笑漫剧</small></span>
      </a>
      <nav aria-label="页面导航">
        <a href="#release-notes">更新记录</a>
        <a href="/fastaiui/recharge">充值中心</a>
        <a class="md-help-link" href="https://fastaiapi.cloud/help/codex/" target="_blank" rel="noopener noreferrer">帮助手册 <span aria-hidden="true">↗</span></a>
      </nav>
    </header>

    <main class="md-main">
      <section class="md-hero" aria-labelledby="download-title">
        <div class="md-hero-copy">
          <span class="md-eyebrow"><span class="md-dot"></span> 笑笑漫剧 · AI 影像创作工作台</span>
          <h1 id="download-title">让脑海中的故事，<br><em>成为眼前的影像。</em></h1>
          <p class="md-intro">从一句灵感，到一部作品。<br>在一个工作台里，完成剧本、角色、分镜与视频创作。</p>
          <div class="md-hero-actions">
            <a class="md-button md-button-dark" href="#downloads">下载客户端 <span aria-hidden="true">↓</span></a>
            <a class="md-text-link" href="#release-notes">看看本次更新 <span aria-hidden="true">↗</span></a>
          </div>
          <div class="md-hero-meta">
            <span v-if="current" class="md-version-pill"><span class="md-dot"></span> v{{ current.version }} 稳定版</span>
            <span v-else>{{ loading ? '正在获取最新版本' : '等待正式版本发布' }}</span>
            <span>支持 macOS、Windows 与 Android</span>
          </div>
        </div>

        <div class="md-story-art" aria-hidden="true">
          <div class="md-art-orbit"></div>
          <div class="md-art-caption"><span>YOUR STORY, IN MOTION</span><span>01 / 03</span></div>
          <div class="md-film-frame md-film-back"><span>角色设定</span><i></i></div>
          <div class="md-film-frame md-film-main">
            <div class="md-frame-label"><span>SCENE 01</span><span>16:9</span></div>
            <svg class="md-landscape" viewBox="0 0 520 300" fill="none">
              <defs><linearGradient id="md-sky" x1="260" y1="0" x2="260" y2="300" gradientUnits="userSpaceOnUse"><stop stop-color="#d7e3c8"/><stop offset="1" stop-color="#f1efd3"/></linearGradient></defs>
              <rect width="520" height="300" rx="12" fill="url(#md-sky)"/>
              <circle cx="358" cy="88" r="43" fill="#f8f4cf"/>
              <path d="M0 206C70 201 87 133 158 142C221 150 227 199 290 187C348 175 367 121 430 147C471 164 488 190 520 182V300H0Z" fill="#95b19b"/>
              <path d="M0 255C66 253 109 195 175 208C254 223 288 271 348 237C414 199 452 203 520 227V300H0Z" fill="#446f5d"/>
              <path d="M0 280C81 249 145 278 210 272C315 261 354 253 434 285L520 277V300H0Z" fill="#254b3e"/>
              <path d="M268 300C278 275 305 269 322 252C339 234 330 223 339 213" stroke="#dfd5a8" stroke-width="8"/>
            </svg>
            <span class="md-art-play"><svg viewBox="0 0 24 24"><path d="m9 6 9 6-9 6z" fill="currentColor"/></svg></span>
            <div class="md-frame-footer"><span>每个故事，都值得被看见。</span><span>00:00:15</span></div>
          </div>
          <div class="md-film-note"><span class="md-note-mark">✦</span><span>灵感，就从这一帧开始。<small>从文字，到流动的画面</small></span></div>
          <div class="md-art-track"><span>剧本</span><i></i><span>角色</span><i></i><span>分镜</span><i></i><strong>成片 <span>↗</span></strong></div>
        </div>
      </section>

      <section id="downloads" class="md-download-section" aria-labelledby="download-section-title">
        <div class="md-section-heading">
          <div><span class="md-eyebrow">GET STARTED</span><h2 id="download-section-title">选好设备，即刻开拍。</h2></div>
          <p>下载适合你设备的版本，开启下一部作品。</p>
        </div>
        <p v-if="loading" class="md-state" role="status">正在获取最新安装包…</p>
        <div v-if="error" class="md-state md-error" role="alert"><strong>{{ installers.length ? '操作未完成' : '暂时无法读取版本信息' }}</strong><p>{{ error }}</p><p v-if="!installers.length">请稍后刷新页面重试。</p></div>
        <div v-if="installers.length || androidRelease.published" class="md-download-grid">
          <article v-for="asset in installers" :key="asset.name" class="md-download-card">
            <div class="md-card-top">
              <span class="md-platform-icon" :class="{ 'md-windows-icon': asset.platform === 'windows' }" aria-hidden="true">
                <svg v-if="asset.platform === 'macos'" viewBox="0 0 32 32" fill="none"><rect x="3.5" y="5.5" width="25" height="17" rx="3" stroke="currentColor" stroke-width="1.7"/><path d="M11 27h10M16 23v4" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"/></svg>
                <svg v-else viewBox="0 0 32 32" fill="currentColor"><path d="M4 6.5 14.7 5v10H4zm12.5-1.8L28 3v12H16.5zM4 17h10.7v10L4 25.5zm12.5 0H28v12l-11.5-1.7z"/></svg>
              </span>
              <span class="md-card-version">v{{ downloads!.version }}</span>
            </div>
            <h3>{{ asset.platform === 'macos' ? 'macOS' : 'Windows' }}</h3>
            <p class="md-card-subtitle">{{ architecture(asset) }}</p>
            <div class="md-file-meta"><span>{{ asset.platform === 'macos' ? 'DMG 安装包' : asset.name.toLowerCase().endsWith('.msi') ? 'MSI 安装包' : 'EXE 安装包' }}</span><span>{{ fileSize(asset.size_bytes) }}</span><span>官方稳定版</span></div>
            <a class="md-button md-download-button" :href="asset.url" :aria-label="`下载 ${architecture(asset)} v${downloads!.version} 安装包`">下载 {{ asset.platform === 'macos' ? 'macOS' : 'Windows' }} 版 <svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M12 3v12m-4-4 4 4 4-4M5 17v4h14v-4" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"/></svg></a>
            <div class="md-card-bottom"><span>官方发布 · SHA-256 校验</span><a v-if="alternateDownload(asset.url)" :href="alternateDownload(asset.url)">备用线路下载 <span aria-hidden="true">↗</span></a></div>
            <details class="md-file-check"><summary>查看文件校验信息 <span aria-hidden="true">＋</span></summary><div><code>{{ asset.sha256 }}</code><button type="button" :aria-label="`复制 ${architecture(asset)} SHA-256`" @click="$emit('copy', asset.sha256)">复制 SHA-256</button></div></details>
          </article>
          <article v-if="androidRelease.published" class="md-download-card md-android-card">
            <div class="md-card-top">
              <span class="md-platform-icon" aria-hidden="true"><svg viewBox="0 0 32 32" fill="none"><rect x="8" y="2.5" width="16" height="27" rx="3.5" stroke="currentColor" stroke-width="1.7"/><path d="M13 6h6M14 25h4" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"/></svg></span>
              <span class="md-card-version">v{{ androidRelease.version }}</span>
            </div>
            <h3>Android</h3>
            <p class="md-card-subtitle">适用于 Android 8.0 及以上 · 手机与平板</p>
            <div class="md-file-meta"><span>APK 安装包</span><span>{{ fileSize(androidRelease.size_bytes) }}</span><span>正式版 · 需要联网</span></div>
            <a class="md-button md-download-button" :href="androidRelease.url" :aria-label="`下载 Android v${androidRelease.version} 安装包`">下载 Android 版 <svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M12 3v12m-4-4 4 4 4-4M5 17v4h14v-4" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"/></svg></a>
            <div v-if="androidQrCode" class="md-android-qr">
              <a :href="androidRelease.url" aria-label="扫码或点击下载 Android 安装包"><img :src="androidQrCode" width="192" height="192" alt="扫描二维码下载 Android 正式安装包"></a>
              <div><strong>手机扫码下载</strong><p>使用安卓手机扫码，下载 v{{ androidRelease.version }} 正式安装包。</p></div>
            </div>
            <div class="md-card-bottom"><span>官方发布 · 覆盖安装保留本机记录</span></div>
            <details class="md-file-check"><summary>查看文件校验信息 <span aria-hidden="true">＋</span></summary><div><code>{{ androidRelease.sha256 }}</code><button type="button" aria-label="复制 Android SHA-256" @click="$emit('copy', androidRelease.sha256)">复制 SHA-256</button></div></details>
          </article>
        </div>
        <div v-else-if="!loading && !error" class="md-state"><h3>安装包准备中</h3><p>正式版本发布后，安装包与更新说明将在这里同步展示。</p></div>
        <p class="md-install-tip"><svg viewBox="0 0 20 20" fill="none" aria-hidden="true"><path d="m10 2 6 2v5c0 4-6 8-6 8S4 13 4 9V4z" stroke="currentColor" stroke-width="1.4"/><path d="m7 9 2 2 4-4" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/></svg>已有用户请先保存当前项目，再覆盖安装新版。安卓升级请直接覆盖安装，保留应用数据及原图片、视频和项目记录；卸载或清除应用数据前请备份项目与素材。</p>
      </section>

      <section v-if="current" id="release-notes" class="md-release-section" aria-labelledby="release-notes-title">
        <div class="md-release-heading"><span class="md-eyebrow">WHAT'S NEW</span><h2 id="release-notes-title">每次更新，<br>让创作更进一步。</h2><span class="md-version-pill"><span class="md-dot"></span> v{{ current.version }} · 稳定版</span></div>
        <div class="md-release-content"><div class="md-release-title"><h3>版本更新说明</h3><span>LATEST RELEASE</span></div><ul class="md-release-list"><li v-for="(paragraph, index) in releaseNotes" :key="index"><span class="md-release-bullet" aria-hidden="true"></span><p>{{ paragraph }}</p></li></ul></div>
      </section>

      <section class="md-support"><div><span class="md-support-symbol" aria-hidden="true">↗</span><div><h2>创作路上，我们都在。</h2><p>安装或使用遇到问题？随时联系我们。</p></div></div><p>QQ <strong>172733456</strong><span></span>微信 <strong>selectthree</strong></p></section>
    </main>
    <footer class="md-footer"><span>FastAiApi <span>／</span> 笑笑漫剧</span><p>从灵感出发，让故事发生。</p><a href="#download-title">回到顶部 ↑</a></footer>
    <div v-if="notice" class="md-copy-notice" role="status"><span>{{ notice }}</span><button type="button" aria-label="关闭提示" @click="$emit('dismiss')">×</button></div>
  </div>
</template>
