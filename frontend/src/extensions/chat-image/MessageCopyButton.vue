<template>
  <button
    class="message-copy-button"
    type="button"
    :title="copied ? '已复制' : '复制回复'"
    :aria-label="copied ? '已复制' : '复制回复'"
    @click="copyContent"
  >
    <Icon :name="copied ? 'check' : 'copy'" size="sm" />
    <span>{{ copied ? '已复制' : '复制' }}</span>
  </button>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  content: string
}>()

const copied = ref(false)
let resetTimer: ReturnType<typeof setTimeout> | undefined

function fallbackCopy(value: string): boolean {
  const textarea = document.createElement('textarea')
  textarea.value = value
  textarea.setAttribute('readonly', 'true')
  textarea.style.cssText = 'position:fixed;left:0;top:0;width:1px;height:1px;opacity:0;pointer-events:none'
  document.body.appendChild(textarea)
  textarea.focus({ preventScroll: true })
  textarea.select()
  textarea.setSelectionRange(0, value.length)
  try {
    return document.execCommand('copy')
  } finally {
    document.body.removeChild(textarea)
  }
}

async function copyContent(): Promise<void> {
  if (!props.content) return

  let success = false
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(props.content)
      success = true
    } else {
      success = fallbackCopy(props.content)
    }
  } catch {
    success = fallbackCopy(props.content)
  }

  if (!success) return
  copied.value = true
  if (resetTimer) clearTimeout(resetTimer)
  resetTimer = setTimeout(() => {
    copied.value = false
    resetTimer = undefined
  }, 2000)
}

onBeforeUnmount(() => {
  if (resetTimer) clearTimeout(resetTimer)
})
</script>

<style scoped>
.message-copy-button {
  display: inline-flex;
  min-height: 32px;
  align-items: center;
  gap: 6px;
  margin-top: 7px;
  padding: 0 9px;
  border: 1px solid transparent;
  border-radius: 7px;
  background: transparent;
  color: rgb(107 114 128);
  font-size: 12px;
  cursor: pointer;
}

.message-copy-button:hover {
  border-color: rgb(209 213 219);
  background: rgb(243 244 246);
  color: rgb(17 24 39);
}

:global(.dark .chat-image-shell .message-copy-button) {
  color: rgb(156 163 175);
}

:global(.dark .chat-image-shell .message-copy-button:hover) {
  border-color: rgb(75 85 99);
  background: rgb(31 41 55);
  color: rgb(243 244 246);
}
</style>
