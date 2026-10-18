<template>
  <Teleport to="body">
    <div class="image-lightbox" role="dialog" aria-modal="true" aria-label="图片原图预览" @click.self="emit('close')">
      <button class="lightbox-close" type="button" title="关闭预览" @click="emit('close')">
        <Icon name="x" />
      </button>
      <button
        v-if="sources.length > 1"
        class="lightbox-nav previous"
        type="button"
        title="上一张"
        @click="move(-1)"
      >
        <Icon name="chevronLeft" />
      </button>
      <div class="lightbox-content">
        <img :key="currentURL" :src="currentURL" :alt="`生成结果 ${index + 1}`" referrerpolicy="no-referrer" />
        <div class="lightbox-info">
          <span v-if="sources.length > 1">{{ index + 1 }} / {{ sources.length }}</span>
          <button type="button" @click="copyURL">
            <Icon :name="copied ? 'check' : 'copy'" size="sm" />{{ copied ? '已复制' : '复制链接' }}
          </button>
          <a :href="currentURL" target="_blank" rel="noopener noreferrer" referrerpolicy="no-referrer">
            <Icon name="externalLink" size="sm" />新窗口打开
          </a>
        </div>
      </div>
      <button
        v-if="sources.length > 1"
        class="lightbox-nav next"
        type="button"
        title="下一张"
        @click="move(1)"
      >
        <Icon name="chevronRight" />
      </button>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { normalizeImageURL } from './imageUrl'

const props = defineProps<{
  sources: string[]
  index: number
}>()

const emit = defineEmits<{
  close: []
  'update:index': [index: number]
}>()

const copied = ref(false)
const currentURL = computed(() => normalizeImageURL(props.sources[props.index] || ''))

function move(offset: number): void {
  if (props.sources.length < 2) return
  emit('update:index', (props.index + offset + props.sources.length) % props.sources.length)
}

async function copyURL(): Promise<void> {
  if (!currentURL.value) return
  await navigator.clipboard.writeText(currentURL.value)
  copied.value = true
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') emit('close')
  else if (event.key === 'ArrowLeft') move(-1)
  else if (event.key === 'ArrowRight') move(1)
}

watch(() => props.index, () => { copied.value = false })
onMounted(() => window.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))
</script>

<style scoped>
.image-lightbox { position: fixed; z-index: 100; inset: 0; display: flex; align-items: center; justify-content: center; padding: 56px 72px 28px; background: rgb(0 0 0 / .78); backdrop-filter: blur(10px); }
.lightbox-content { display: flex; max-width: 100%; max-height: 100%; flex-direction: column; align-items: center; }
.lightbox-content img { display: block; width: auto; max-width: min(88vw, 1280px); max-height: calc(100vh - 140px); border-radius: 8px; object-fit: contain; }
.lightbox-close, .lightbox-nav { position: fixed; display: inline-flex; width: 42px; height: 42px; align-items: center; justify-content: center; border: 1px solid rgb(255 255 255 / .35); border-radius: 50%; background: rgb(0 0 0 / .28); color: white; cursor: pointer; }
.lightbox-close { top: 20px; right: 22px; }
.lightbox-nav { top: 50%; transform: translateY(-50%); }
.lightbox-nav.previous { left: 18px; }
.lightbox-nav.next { right: 18px; }
.lightbox-close:hover, .lightbox-nav:hover { background: rgb(255 255 255 / .16); }
.lightbox-info { display: flex; min-width: 0; flex-wrap: wrap; align-items: center; justify-content: center; gap: 9px; margin-top: 12px; color: rgb(255 255 255 / .78); font-size: 12px; }
.lightbox-info button, .lightbox-info a { display: inline-flex; min-height: 32px; align-items: center; gap: 5px; padding: 0 10px; border: 1px solid rgb(255 255 255 / .35); border-radius: 7px; background: transparent; color: white; font-size: 12px; text-decoration: none; cursor: pointer; }
.lightbox-info button:hover, .lightbox-info a:hover { background: rgb(255 255 255 / .12); }
@media (max-width: 640px) {
  .image-lightbox { padding: 54px 12px 20px; }
  .lightbox-content img { max-width: calc(100vw - 24px); max-height: calc(100vh - 150px); }
  .lightbox-nav { top: auto; bottom: 18px; }
  .lightbox-nav.previous { left: 14px; }
  .lightbox-nav.next { right: 14px; }
}
</style>
