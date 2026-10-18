<template>
  <div class="prompt-backdrop" @click.self="emit('close')">
    <section class="prompt-modal" role="dialog" aria-modal="true" aria-label="提示词库">
      <header>
        <div>
          <h2>提示词库</h2>
          <p>{{ subtitle }}</p>
        </div>
        <button class="prompt-icon-button" title="关闭" @click="emit('close')"><Icon name="x" /></button>
      </header>

      <div class="prompt-body" @scroll.passive="onScroll">
        <div class="prompt-filters">
          <label class="prompt-search">
            <Icon name="search" size="sm" />
            <input v-model="keyword" placeholder="搜索标题、分类、词源或提示词" />
          </label>
          <select v-model="sourceFilter" aria-label="提示词来源">
            <option value="all">全部来源</option>
            <option v-for="source in sources" :key="source.id" :value="source.id">{{ source.name }}</option>
          </select>
          <select v-model="categoryFilter" aria-label="提示词分类">
            <option value="all">全部分类</option>
            <option v-for="category in categories" :key="category" :value="category">{{ category }}</option>
          </select>
        </div>

        <div v-if="loading" class="prompt-state">正在读取本地提示词...</div>
        <div v-else-if="error" class="prompt-state prompt-error">
          <span>{{ error }}</span>
          <button @click="fetchLibrary(true)">重新加载</button>
        </div>
        <div v-else-if="filtered.length === 0" class="prompt-state">没有匹配的提示词</div>
        <div v-else class="prompt-grid">
          <article v-for="item in visible" :key="item.id" class="prompt-card" @click="select(item)">
            <div class="prompt-media" :class="{ empty: !previewFor(item) }">
              <img
                v-if="previewFor(item)"
                :src="previewFor(item)"
                :alt="item.title"
                loading="lazy"
                decoding="async"
                @error="markPreviewFailed(item)"
              />
              <Icon v-else name="sparkles" size="lg" />
            </div>
            <div class="prompt-card-body">
              <div class="prompt-meta">
                <span v-if="promptCategory(item)">{{ promptCategory(item) }}</span>
                <span>{{ item.source_name }}</span>
              </div>
              <h3>{{ item.title }}</h3>
              <p>{{ promptSummary(item) }}</p>
              <footer>
                <small>{{ promptLength(item.prompt) }} 字</small>
                <button @click.stop="select(item)">使用</button>
              </footer>
            </div>
          </article>
        </div>
      </div>

      <footer class="prompt-footer">
        <span>已显示 {{ visible.length }} / {{ filtered.length }} 条，库内 {{ items.length }} 条</span>
        <button v-if="hasMore" @click="visibleLimit = filtered.length">加载全部</button>
      </footer>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import {
  filterPrompts,
  loadPromptLibrary,
  promptCategory,
  promptSummary,
  type PromptLibraryItem,
  type PromptLibrarySource
} from './promptLibrary'

const emit = defineEmits<{
  close: []
  select: [item: PromptLibraryItem]
}>()

const PAGE_SIZE = 24
const loading = ref(false)
const error = ref('')
const items = ref<PromptLibraryItem[]>([])
const sources = ref<PromptLibrarySource[]>([])
const keyword = ref('')
const sourceFilter = ref('all')
const categoryFilter = ref('all')
const visibleLimit = ref(PAGE_SIZE)
const failedPreviews = ref(new Set<string>())

const scopedItems = computed(() => filterPrompts(items.value, '', sourceFilter.value, 'all'))
const categories = computed(() => Array.from(new Set(scopedItems.value.map(promptCategory).filter(Boolean))).sort((a, b) => a.localeCompare(b, 'zh-CN')))
const filtered = computed(() => filterPrompts(items.value, keyword.value, sourceFilter.value, categoryFilter.value))
const visible = computed(() => filtered.value.slice(0, visibleLimit.value))
const hasMore = computed(() => visible.value.length < filtered.value.length)
const subtitle = computed(() => loading.value && !items.value.length
  ? '正在读取本地快照'
  : `${items.value.length} 条提示词，${sources.value.length} 个启用词源`)

async function fetchLibrary(force = false): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const library = await loadPromptLibrary(force)
    items.value = library.items
    sources.value = library.sources.filter((source) => source.enabled)
  } catch (reason: any) {
    error.value = reason?.message || '提示词加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

function select(item: PromptLibraryItem): void {
  emit('select', item)
}

function previewFor(item: PromptLibraryItem): string {
  const candidates = [item.preview, ...item.reference_image_urls]
  return candidates.find((url) => url && !failedPreviews.value.has(url)) || ''
}

function markPreviewFailed(item: PromptLibraryItem): void {
  const url = previewFor(item)
  if (url) failedPreviews.value = new Set([...failedPreviews.value, url])
}

function promptLength(value: string): number {
  return Array.from(value).length
}

function onScroll(event: Event): void {
  const element = event.currentTarget as HTMLElement
  if (!hasMore.value || element.scrollHeight - element.scrollTop - element.clientHeight > 240) return
  visibleLimit.value = Math.min(filtered.value.length, visibleLimit.value + PAGE_SIZE)
}

watch(sourceFilter, () => { categoryFilter.value = 'all' })
watch([keyword, sourceFilter, categoryFilter], () => { visibleLimit.value = PAGE_SIZE })
onMounted(() => fetchLibrary())
</script>

<style scoped>
.prompt-backdrop { position: fixed; inset: 0; z-index: 90; display: flex; align-items: center; justify-content: center; padding: 24px; background: rgb(0 0 0 / .68); }
.prompt-modal { display: grid; width: min(1120px, 100%); height: min(820px, 90vh); grid-template-rows: auto minmax(0, 1fr) auto; overflow: hidden; border: 1px solid rgb(209 213 219); border-radius: 8px; background: white; color: rgb(17 24 39); }
:global(.dark .prompt-modal) { border-color: rgb(75 85 99); background: rgb(17 24 39); color: rgb(243 244 246); }
.prompt-modal > header { display: flex; align-items: center; justify-content: space-between; padding: 18px 20px; border-bottom: 1px solid rgb(229 231 235); }
:global(.dark .prompt-modal > header), :global(.dark .prompt-footer) { border-color: rgb(55 65 81); }
.prompt-modal h2 { margin: 0; font-size: 18px; }
.prompt-modal header p { margin: 4px 0 0; color: rgb(107 114 128); font-size: 13px; }
.prompt-icon-button { display: inline-flex; width: 38px; height: 38px; align-items: center; justify-content: center; border: 1px solid rgb(209 213 219); border-radius: 8px; background: transparent; color: inherit; cursor: pointer; }
.prompt-body { min-height: 0; overflow-y: auto; padding: 16px; background: rgb(249 250 251); }
:global(.dark .prompt-body) { background: rgb(12 18 29); }
.prompt-filters { position: sticky; z-index: 2; top: -16px; display: grid; grid-template-columns: minmax(240px, 1fr) 220px 220px; gap: 10px; margin: -1px 0 14px; padding: 10px; border: 1px solid rgb(209 213 219); border-radius: 8px; background: white; }
:global(.dark .prompt-filters) { border-color: rgb(75 85 99); background: rgb(17 24 39); }
.prompt-search { display: flex; height: 40px; align-items: center; gap: 8px; padding: 0 12px; border: 1px solid rgb(209 213 219); border-radius: 7px; color: rgb(107 114 128); }
.prompt-search input, .prompt-filters select { min-width: 0; width: 100%; border: 0; outline: 0; background: transparent; color: inherit; font-size: 14px; }
.prompt-filters select { height: 40px; padding: 0 10px; border: 1px solid rgb(209 213 219); border-radius: 7px; }
:global(.dark .prompt-search), :global(.dark .prompt-filters select) { border-color: rgb(75 85 99); }
.prompt-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.prompt-card { display: flex; min-width: 0; overflow: hidden; flex-direction: column; border: 1px solid rgb(229 231 235); border-radius: 8px; background: white; cursor: pointer; }
:global(.dark .prompt-card) { border-color: rgb(55 65 81); background: rgb(23 31 43); }
.prompt-card:hover { border-color: rgb(107 114 128); transform: translateY(-1px); }
.prompt-media { display: flex; aspect-ratio: 4 / 3; align-items: center; justify-content: center; overflow: hidden; background: rgb(229 231 235); color: rgb(107 114 128); }
:global(.dark .prompt-media) { background: rgb(31 41 55); }
.prompt-media img { width: 100%; height: 100%; object-fit: cover; }
.prompt-card-body { display: flex; min-height: 178px; flex: 1; flex-direction: column; gap: 8px; padding: 13px; }
.prompt-meta { display: flex; min-height: 22px; flex-wrap: wrap; gap: 5px; }
.prompt-meta span { padding: 3px 7px; border: 1px solid rgb(209 213 219); border-radius: 999px; color: rgb(75 85 99); font-size: 11px; }
:global(.dark .prompt-meta span) { border-color: rgb(75 85 99); color: rgb(209 213 219); }
.prompt-card h3 { display: -webkit-box; margin: 0; overflow: hidden; -webkit-box-orient: vertical; -webkit-line-clamp: 2; font-size: 15px; }
.prompt-card p { display: -webkit-box; margin: 0; overflow: hidden; -webkit-box-orient: vertical; -webkit-line-clamp: 3; color: rgb(107 114 128); font-size: 13px; line-height: 1.55; }
.prompt-card footer { display: flex; align-items: center; justify-content: space-between; margin-top: auto; color: rgb(107 114 128); }
.prompt-card footer button, .prompt-footer button, .prompt-state button { min-height: 34px; padding: 0 14px; border: 0; border-radius: 7px; background: rgb(17 24 39); color: white; cursor: pointer; }
:global(.dark .prompt-card footer button), :global(.dark .prompt-footer button), :global(.dark .prompt-state button) { background: white; color: rgb(17 24 39); }
.prompt-state { display: flex; min-height: 300px; align-items: center; justify-content: center; gap: 14px; color: rgb(107 114 128); }
.prompt-error { color: rgb(185 28 28); }
.prompt-footer { display: flex; min-height: 58px; align-items: center; justify-content: space-between; gap: 12px; padding: 10px 18px; border-top: 1px solid rgb(229 231 235); color: rgb(107 114 128); font-size: 13px; }
@media (max-width: 900px) { .prompt-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .prompt-filters { grid-template-columns: 1fr 1fr; } .prompt-search { grid-column: 1 / -1; } }
@media (max-width: 640px) { .prompt-backdrop { padding: 10px; } .prompt-modal { height: 94vh; } .prompt-grid, .prompt-filters { grid-template-columns: 1fr; } .prompt-search { grid-column: auto; } .prompt-footer span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; } }
</style>
