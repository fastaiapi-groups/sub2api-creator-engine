<script setup lang="ts">
import ChatImageView from './extensions/chat-image/ChatImageView.vue'
import RechargeView from './extensions/recharge/RechargeView.vue'
const recharge = /^\/fastaiui\/recharge\/?$/.test(window.location.pathname)
</script>
<template><RechargeView v-if="recharge"/><ChatImageView v-else/></template>
