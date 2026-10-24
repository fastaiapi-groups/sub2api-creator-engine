<script setup lang="ts">
import ChatImageView from './extensions/chat-image/ChatImageView.vue'
import RechargeView from './extensions/recharge/RechargeView.vue'
import ClientReleases from './extensions/client-updates/ClientReleases.vue'
const clientReleases = /^\/fastaiui\/(admin\/client-releases|downloads\/codexplus)\/?$/.test(window.location.pathname)
const recharge = /^\/fastaiui\/recharge\/?$/.test(window.location.pathname)
</script>
<template><ClientReleases v-if="clientReleases"/><RechargeView v-else-if="recharge"/><ChatImageView v-else/></template>
