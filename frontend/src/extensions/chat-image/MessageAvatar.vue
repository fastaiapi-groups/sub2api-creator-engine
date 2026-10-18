<template>
  <span
    v-if="role === 'assistant'"
    class="message-avatar message-avatar-ai"
    role="img"
    aria-label="GPT 助手"
    title="GPT 助手"
  >
    <img :src="gptLogo" alt="" />
  </span>

  <span
    v-else
    class="message-avatar message-avatar-user"
    role="img"
    aria-label="用户头像"
    title="FastAiApi"
  >
    <img :src="fastAiApiLogo" alt="" />
  </span>
</template>

<script setup lang="ts">
import fastAiApiLogo from './assets/fastapapi-logo.svg'
import gptLogo from './assets/gpt-logo.svg'

defineProps<{
  role: 'user' | 'assistant'
}>()
</script>

<style scoped>
.message-avatar {
  position: relative;
  display: inline-flex;
  width: 36px;
  height: 36px;
  flex: 0 0 36px;
  align-items: center;
  justify-content: center;
  overflow: visible;
  border-radius: 10px;
}

.message-avatar-user {
  overflow: hidden;
  border: 1px solid rgb(45 212 191 / .45);
  background: rgb(17 24 39);
  box-shadow: 0 2px 8px rgb(15 118 110 / .10);
}

.message-avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.message-avatar-ai {
  overflow: hidden;
  border: 1px solid rgb(31 41 55);
  background: rgb(17 24 39);
  color: white;
  box-shadow: 0 3px 10px rgb(17 24 39 / .18);
}

.message-avatar-ai img {
  width: 21px;
  height: 21px;
  object-fit: contain;
  filter: invert(1);
}

:global(.dark .message-avatar-user) {
  border-color: rgb(45 212 191 / .58);
  background: rgb(17 24 39);
  box-shadow: 0 3px 10px rgb(0 0 0 / .18);
}

:global(.dark .message-avatar-ai) {
  border-color: rgb(71 85 105);
  background: rgb(30 41 59);
  color: white;
  box-shadow: 0 3px 10px rgb(0 0 0 / .25);
}
</style>
