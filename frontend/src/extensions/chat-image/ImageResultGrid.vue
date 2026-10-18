<template>
  <div
    class="image-result-grid"
    :class="{ single: totalSlots === 1, multi: totalSlots > 1 }"
    :style="gridStyle"
  >
    <figure v-for="(source, index) in normalizedSources" :key="`${source}-${index}`" class="result-item">
      <button class="result-media has-image" type="button" @click="emit('preview', index)">
        <img
          :src="source"
          :alt="`生成结果 ${index + 1}`"
          loading="lazy"
          decoding="async"
          referrerpolicy="no-referrer"
        />
      </button>
      <figcaption>
        <span v-if="totalSlots > 1">结果 {{ index + 1 }}</span>
        <span class="result-actions">
          <button type="button" title="引用到输入框" @click="emit('reference', index)">
            <Icon name="link" size="sm" />引用
          </button>
          <button type="button" title="站内查看原图" @click="emit('preview', index)">
            <Icon name="eye" size="sm" />查看原图
          </button>
        </span>
      </figcaption>
    </figure>

    <figure v-for="slot in pendingCount" :key="`pending-${slot}`" class="result-item">
      <div class="result-media pending-media">
        <span class="result-spinner"></span>
        <strong>正在处理图片</strong>
        <span>{{ pendingLabel || '图片渠道处理中' }}</span>
        <small v-if="taskId">任务 {{ shortTaskId(taskId) }}</small>
      </div>
      <figcaption v-if="totalSlots > 1"><span>图片 {{ normalizedSources.length + slot }}</span></figcaption>
    </figure>
  </div>
</template>

<script setup lang="ts">
import { computed, type CSSProperties } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { normalizeImageURL } from './imageUrl'

const props = withDefaults(defineProps<{
  sources: string[]
  pendingCount?: number
  pendingLabel?: string
  taskId?: string
  size?: string
}>(), {
  pendingCount: 0,
  pendingLabel: '',
  taskId: '',
  size: 'auto'
})

const emit = defineEmits<{
  preview: [index: number]
  reference: [index: number]
}>()

const normalizedSources = computed(() => props.sources.map(normalizeImageURL))
const totalSlots = computed(() => Math.max(1, normalizedSources.value.length + props.pendingCount))
const gridStyle = computed<CSSProperties>(() => ({
  '--image-result-columns': String(Math.min(2, totalSlots.value)),
  '--image-result-aspect': aspectRatio(props.size)
}))

function aspectRatio(size: string): string {
  const match = /^(\d+)x(\d+)$/i.exec(size)
  if (!match) return '1 / 1'
  const width = Number(match[1])
  const height = Number(match[2])
  return width > 0 && height > 0 ? `${width} / ${height}` : '1 / 1'
}

function shortTaskId(taskId: string): string {
  return taskId.length > 18 ? `${taskId.slice(0, 10)}...${taskId.slice(-5)}` : taskId
}
</script>

<style scoped>
.image-result-grid { display: grid; width: min(416px, 100%); max-width: 100%; grid-template-columns: repeat(var(--image-result-columns), minmax(0, 1fr)); gap: 10px; }
.image-result-grid.single { width: min(288px, 100%); grid-template-columns: minmax(0, 1fr); }
.result-item { width: 100%; min-width: 0; margin: 0; }
.result-media { display: flex; width: 100%; min-width: 0; aspect-ratio: var(--image-result-aspect); align-items: center; justify-content: center; overflow: hidden; border: 1px solid rgb(209 213 219); border-radius: 8px; background: rgb(243 244 246); color: inherit; }
:global(.dark .image-result-grid .result-media) { border-color: rgb(55 65 81); background: rgb(23 31 43); }
.result-media.has-image { aspect-ratio: auto; padding: 0; cursor: zoom-in; }
.result-media img { display: block; width: 100%; height: auto; max-width: 100%; border-radius: 8px; object-fit: contain; }
.single .result-media img { width: auto; max-width: 100%; max-height: min(56vh, 384px); }
.pending-media { min-width: 0; min-height: 220px; flex-direction: column; gap: 9px; color: rgb(107 114 128); }
.pending-media strong { color: rgb(17 24 39); font-size: 14px; }
:global(.dark .image-result-grid .pending-media strong) { color: rgb(243 244 246); }
.pending-media small { max-width: calc(100% - 24px); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 11px; }
.result-spinner { width: 26px; height: 26px; border: 3px solid rgb(209 213 219); border-top-color: rgb(17 24 39); border-radius: 50%; animation: result-spin .9s linear infinite; }
:global(.dark .image-result-grid .result-spinner) { border-color: rgb(75 85 99); border-top-color: white; }
figcaption { display: flex; min-height: 38px; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 6px 10px; padding: 6px 2px 0; color: rgb(107 114 128); font-size: 12px; }
.result-actions { display: flex; min-width: 0; flex-wrap: wrap; justify-content: flex-end; gap: 5px; margin-left: auto; }
figcaption button { display: inline-flex; min-height: 30px; align-items: center; gap: 4px; padding: 0 7px; border: 1px solid rgb(209 213 219); border-radius: 7px; background: transparent; color: inherit; cursor: pointer; }
figcaption button:hover { border-color: rgb(107 114 128); color: rgb(17 24 39); }
:global(.dark .image-result-grid figcaption button) { border-color: rgb(75 85 99); }
:global(.dark .image-result-grid figcaption button:hover) { color: rgb(243 244 246); }
@keyframes result-spin { to { transform: rotate(360deg); } }
@media (max-width: 640px) {
  .image-result-grid { width: min(100%, 416px); }
  .image-result-grid.single { width: min(100%, 264px); }
}
</style>
