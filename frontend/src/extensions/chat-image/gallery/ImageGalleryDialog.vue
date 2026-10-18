<template>
  <Teleport to="body">
    <div v-if="show" class="gallery-overlay" @click.self="close">
      <section class="gallery-dialog" role="dialog" aria-modal="true" aria-label="历史图片">
        <header class="gallery-header">
          <div>
            <h2>历史图片</h2>
            <p>共 {{ total }} 张，已选择 {{ selectedIds.length }} 张</p>
          </div>
          <button class="icon-button" type="button" title="关闭图库" @click="close">
            <Icon name="x" />
          </button>
        </header>

        <div class="gallery-toolbar">
          <label class="select-page">
            <input
              type="checkbox"
              :checked="allPageSelected"
              :disabled="!items.length"
              @change="toggleCurrentPage"
            />
            选择当前页
          </label>
          <span v-if="selectionError" class="toolbar-error">{{ selectionError }}</span>
          <span class="toolbar-spacer"></span>
          <button type="button" :disabled="!selectedIds.length || downloading" @click="downloadSelected">
            <Icon name="download" size="sm" />
            {{ downloadLabel }}
          </button>
          <button class="danger" type="button" :disabled="!selectedIds.length || deleting" @click="deleteSelected">
            <Icon name="trash" size="sm" />
            {{ deleting ? '删除中' : `删除 (${selectedIds.length})` }}
          </button>
          <button type="button" :disabled="!selectedIds.length" @click="clearSelection">取消选择</button>
        </div>

        <main class="gallery-content">
          <div v-if="loading" class="gallery-state">
            <span class="spinner"></span>
            <strong>正在加载图片</strong>
          </div>
          <div v-else-if="error" class="gallery-state error-state">
            <Icon name="exclamationCircle" size="lg" />
            <strong>图库加载失败</strong>
            <p>{{ error }}</p>
            <button type="button" @click="load(page)">重试</button>
          </div>
          <div v-else-if="!items.length" class="gallery-state">
            <Icon name="grid" size="xl" />
            <strong>还没有生成图片</strong>
          </div>
          <div v-else class="gallery-grid">
            <article
              v-for="(image, index) in items"
              :key="image.id"
              class="gallery-item"
              :class="{ selected: selected.has(image.id) }"
            >
              <label class="image-checkbox" :title="selected.has(image.id) ? '取消选择' : '选择图片'">
                <input
                  type="checkbox"
                  :checked="selected.has(image.id)"
                  @change="toggleImage(image.id)"
                />
              </label>
              <button class="image-preview" type="button" @click="openPreview(index)">
                <img
                  v-if="!failedImages.has(image.id)"
                  :src="normalizeImageURL(image.url)"
                  :alt="image.displayName || `历史图片 ${image.id}`"
                  loading="lazy"
                  decoding="async"
                  referrerpolicy="no-referrer"
                  @error="markFailed(image.id)"
                />
                <span v-else class="broken-image">
                  <Icon name="exclamationCircle" size="lg" />
                  图片链接已失效
                </span>
              </button>
              <div class="image-meta">
                <strong :title="image.displayName || image.conversationName">
                  {{ image.displayName || image.conversationName || `图片 ${image.id}` }}
                </strong>
                <span>{{ image.model || '图片模型' }}</span>
                <span>{{ formatDate(image.createdAt) }}<template v-if="image.sizeBytes"> · {{ formatBytes(image.sizeBytes) }}</template></span>
              </div>
            </article>
          </div>
        </main>

        <footer class="gallery-footer">
          <span>第 {{ page }} / {{ pages }} 页</span>
          <div class="pagination-actions">
            <button type="button" :disabled="page <= 1 || loading" @click="load(page - 1)">
              <Icon name="chevronLeft" size="sm" />上一页
            </button>
            <button type="button" :disabled="page >= pages || loading" @click="load(page + 1)">
              下一页<Icon name="chevronRight" size="sm" />
            </button>
          </div>
        </footer>
      </section>
    </div>
  </Teleport>

  <ImageLightbox
    v-if="previewImages.length"
    :sources="previewImages"
    :index="previewIndex"
    @update:index="previewIndex = $event"
    @close="closePreview"
  />
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { saveAs } from 'file-saver'
import Icon from '@/components/icons/Icon.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import ImageLightbox from '../ImageLightbox.vue'
import { normalizeImageURL } from '../imageUrl'
import { deleteGalleryImages, downloadGalleryImages, listGalleryImages } from './api'
import type { GalleryDeleteResult, GalleryImage } from './types'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{
  close: []
  deleted: [result: GalleryDeleteResult]
}>()

const PAGE_SIZE = 24
const MAX_SELECTION = 100
const items = ref<GalleryImage[]>([])
const total = ref(0)
const page = ref(1)
const pages = ref(1)
const loading = ref(false)
const downloading = ref(false)
const deleting = ref(false)
const error = ref('')
const selectionError = ref('')
const selected = ref(new Set<number>())
const failedImages = ref(new Set<number>())
const previewImages = ref<string[]>([])
const previewIndex = ref(0)
let listController: AbortController | undefined
let downloadController: AbortController | undefined

const selectedIds = computed(() => Array.from(selected.value))
const allPageSelected = computed(() => (
  items.value.length > 0 && items.value.every((item) => selected.value.has(item.id))
))
const downloadLabel = computed(() => {
  if (downloading.value) return '准备下载'
  if (selectedIds.value.length === 1) return '下载图片'
  return `打包下载 (${selectedIds.value.length})`
})

async function load(targetPage: number): Promise<void> {
  listController?.abort()
  listController = new AbortController()
  loading.value = true
  error.value = ''
  try {
    const result = await listGalleryImages(targetPage, PAGE_SIZE, listController.signal)
    items.value = result.items
    total.value = result.total
    page.value = result.page
    pages.value = result.pages
  } catch (loadError: any) {
    if (loadError?.code !== 'ERR_CANCELED') {
      error.value = extractApiErrorMessage(loadError, '无法加载历史图片')
    }
  } finally {
    loading.value = false
  }
}

function toggleImage(id: number): void {
  const next = new Set(selected.value)
  selectionError.value = ''
  if (next.has(id)) next.delete(id)
  else if (next.size < MAX_SELECTION) next.add(id)
  else selectionError.value = `一次最多选择 ${MAX_SELECTION} 张图片`
  selected.value = next
}

function toggleCurrentPage(event: Event): void {
  const checked = (event.target as HTMLInputElement).checked
  const next = new Set(selected.value)
  selectionError.value = ''
  if (!checked) {
    items.value.forEach((item) => next.delete(item.id))
  } else {
    for (const item of items.value) {
      if (next.size >= MAX_SELECTION) {
        selectionError.value = `一次最多选择 ${MAX_SELECTION} 张图片`
        break
      }
      next.add(item.id)
    }
  }
  selected.value = next
}

function clearSelection(): void {
  selected.value = new Set()
  selectionError.value = ''
}

async function downloadSelected(): Promise<void> {
  if (!selectedIds.value.length || downloading.value) return
  downloadController?.abort()
  downloadController = new AbortController()
  downloading.value = true
  selectionError.value = ''
  try {
    const result = await downloadGalleryImages(selectedIds.value, downloadController.signal)
    saveAs(result.blob, result.filename)
  } catch (downloadError: any) {
    if (downloadError?.code !== 'ERR_CANCELED') {
      selectionError.value = extractApiErrorMessage(downloadError, '图片下载失败')
    }
  } finally {
    downloading.value = false
  }
}

async function deleteSelected(): Promise<void> {
  if (!selectedIds.value.length || deleting.value) return
  if (!window.confirm(`确定删除选中的 ${selectedIds.value.length} 张图片记录吗？渠道中的源文件不会被删除。`)) return
  deleting.value = true
  selectionError.value = ''
  try {
    const result = await deleteGalleryImages(selectedIds.value)
    emit('deleted', result)
    clearSelection()
    const nextPage = items.value.length <= result.deleted && page.value > 1 ? page.value - 1 : page.value
    await load(nextPage)
  } catch (deleteError) {
    selectionError.value = extractApiErrorMessage(deleteError, '删除图片记录失败')
  } finally {
    deleting.value = false
  }
}

function openPreview(index: number): void {
  previewImages.value = items.value.map((item) => normalizeImageURL(item.url))
  previewIndex.value = index
}

function closePreview(): void {
  previewImages.value = []
  previewIndex.value = 0
}

function markFailed(id: number): void {
  failedImages.value = new Set(failedImages.value).add(id)
}

function close(): void {
  if (deleting.value) return
  emit('close')
}

function formatDate(value: string): string {
  return new Date(value).toLocaleString([], {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}

function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / (1024 * 1024)).toFixed(1)} MB`
}

function onEscape(event: KeyboardEvent): void {
  if (event.key === 'Escape' && props.show && !previewImages.value.length) close()
}

watch(() => props.show, (show) => {
  if (show) {
    clearSelection()
    failedImages.value = new Set()
    void load(1)
    document.addEventListener('keydown', onEscape)
  } else {
    listController?.abort()
    downloadController?.abort()
    closePreview()
    document.removeEventListener('keydown', onEscape)
  }
}, { immediate: true })

onBeforeUnmount(() => {
  listController?.abort()
  downloadController?.abort()
  document.removeEventListener('keydown', onEscape)
})
</script>

<style scoped>
.gallery-overlay { position: fixed; inset: 0; z-index: 80; display: flex; align-items: center; justify-content: center; padding: 18px; background: rgb(0 0 0 / 62%); }
.gallery-dialog { display: grid; width: min(1180px, 100%); height: min(880px, calc(100vh - 36px)); grid-template-rows: auto auto minmax(0, 1fr) auto; overflow: hidden; border: 1px solid rgb(209 213 219); border-radius: 8px; background: white; color: rgb(17 24 39); box-shadow: 0 24px 70px rgb(0 0 0 / 28%); }
:global(.dark .gallery-dialog) { border-color: rgb(55 65 81); background: rgb(17 24 39); color: rgb(243 244 246); }
.gallery-header, .gallery-toolbar, .gallery-footer, .pagination-actions { display: flex; align-items: center; }
.gallery-header { justify-content: space-between; padding: 18px 20px 14px; border-bottom: 1px solid rgb(229 231 235); }
:global(.dark .gallery-header), :global(.dark .gallery-toolbar), :global(.dark .gallery-footer) { border-color: rgb(55 65 81); }
.gallery-header h2 { margin: 0; font-size: 18px; }
.gallery-header p { margin: 4px 0 0; color: rgb(107 114 128); font-size: 12px; }
.icon-button { display: inline-flex; width: 36px; height: 36px; align-items: center; justify-content: center; border: 1px solid rgb(209 213 219); border-radius: 8px; background: transparent; color: inherit; cursor: pointer; }
.gallery-toolbar { min-height: 54px; flex-wrap: wrap; gap: 8px; padding: 8px 20px; border-bottom: 1px solid rgb(229 231 235); }
.select-page { display: inline-flex; align-items: center; gap: 7px; font-size: 13px; cursor: pointer; }
.select-page input, .image-checkbox input { width: 16px; height: 16px; accent-color: rgb(17 24 39); }
:global(.dark .select-page input), :global(.dark .image-checkbox input) { accent-color: white; }
.toolbar-spacer { flex: 1; }
.toolbar-error { max-width: 420px; color: rgb(185 28 28); font-size: 12px; }
:global(.dark .toolbar-error) { color: rgb(248 113 113); }
.gallery-toolbar button, .gallery-footer button, .gallery-state button { display: inline-flex; min-height: 34px; align-items: center; justify-content: center; gap: 5px; padding: 0 11px; border: 1px solid rgb(209 213 219); border-radius: 7px; background: transparent; color: inherit; cursor: pointer; }
.gallery-toolbar button:hover:not(:disabled), .gallery-footer button:hover:not(:disabled), .gallery-state button:hover { background: rgb(243 244 246); }
:global(.dark .gallery-toolbar button), :global(.dark .gallery-footer button), :global(.dark .gallery-state button), :global(.dark .gallery-dialog .icon-button) { border-color: rgb(75 85 99); }
:global(.dark .gallery-toolbar button:hover:not(:disabled)), :global(.dark .gallery-footer button:hover:not(:disabled)), :global(.dark .gallery-state button:hover) { background: rgb(31 41 55); }
.gallery-toolbar .danger { color: rgb(185 28 28); }
:global(.dark .gallery-toolbar .danger) { color: rgb(248 113 113); }
button:disabled { cursor: not-allowed; opacity: .45; }
.gallery-content { min-height: 0; overflow-y: auto; padding: 18px 20px; }
.gallery-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 14px; }
.gallery-item { position: relative; min-width: 0; overflow: hidden; border: 1px solid rgb(209 213 219); border-radius: 8px; background: rgb(249 250 251); }
.gallery-item.selected { border-color: rgb(17 24 39); box-shadow: 0 0 0 2px rgb(17 24 39 / 18%); }
:global(.dark .gallery-item) { border-color: rgb(55 65 81); background: rgb(23 31 43); }
:global(.dark .gallery-item.selected) { border-color: white; box-shadow: 0 0 0 2px rgb(255 255 255 / 18%); }
.image-checkbox { position: absolute; top: 8px; left: 8px; z-index: 1; display: inline-flex; width: 28px; height: 28px; align-items: center; justify-content: center; border-radius: 6px; background: rgb(255 255 255 / 88%); cursor: pointer; }
:global(.dark .image-checkbox) { background: rgb(17 24 39 / 88%); }
.image-preview { display: flex; width: 100%; aspect-ratio: 1; align-items: center; justify-content: center; overflow: hidden; border: 0; background: rgb(229 231 235); cursor: zoom-in; }
:global(.dark .image-preview) { background: rgb(12 18 29); }
.image-preview img { display: block; width: 100%; height: 100%; object-fit: cover; }
.broken-image { display: flex; align-items: center; flex-direction: column; gap: 8px; color: rgb(107 114 128); font-size: 12px; }
.image-meta { display: grid; gap: 3px; padding: 10px 11px 12px; }
.image-meta strong { overflow: hidden; font-size: 13px; text-overflow: ellipsis; white-space: nowrap; }
.image-meta span { overflow: hidden; color: rgb(107 114 128); font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.gallery-state { display: flex; min-height: 100%; align-items: center; justify-content: center; flex-direction: column; gap: 10px; color: rgb(107 114 128); text-align: center; }
.gallery-state strong { color: inherit; font-size: 15px; }
.gallery-state p { max-width: 560px; margin: 0; font-size: 12px; }
.error-state { color: rgb(185 28 28); }
.spinner { width: 28px; height: 28px; border: 3px solid rgb(209 213 219); border-top-color: rgb(17 24 39); border-radius: 50%; animation: gallery-spin .8s linear infinite; }
:global(.dark .spinner) { border-color: rgb(75 85 99); border-top-color: white; }
.gallery-footer { justify-content: space-between; padding: 11px 20px; border-top: 1px solid rgb(229 231 235); color: rgb(107 114 128); font-size: 12px; }
.pagination-actions { gap: 8px; }
@keyframes gallery-spin { to { transform: rotate(360deg); } }
@media (max-width: 900px) { .gallery-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
@media (max-width: 640px) {
  .gallery-overlay { padding: 0; }
  .gallery-dialog { width: 100%; height: 100%; border: 0; border-radius: 0; }
  .gallery-header, .gallery-toolbar, .gallery-content, .gallery-footer { padding-right: 14px; padding-left: 14px; }
  .gallery-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 9px; }
  .toolbar-spacer { display: none; }
  .toolbar-error { order: 5; width: 100%; max-width: none; }
  .image-meta { padding: 8px; }
}
</style>
