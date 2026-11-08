<script setup lang="ts">
import ChatImageView from './extensions/chat-image/ChatImageView.vue'
import RechargeView from './extensions/recharge/RechargeView.vue'
import RechargeStats from './extensions/recharge/RechargeStats.vue'
import ClientReleases from './extensions/client-updates/ClientReleases.vue'
const clientReleases = /^\/fastaiui\/(admin\/client-releases|downloads\/(codexplus|manju))\/?$/.test(window.location.pathname)
const recharge = /^\/fastaiui\/recharge\/?$/.test(window.location.pathname)
const rechargeStats = /^\/fastaiui\/admin\/recharge-stats\/?$/.test(window.location.pathname)
</script>
<template><ClientReleases v-if="clientReleases"/><RechargeStats v-else-if="rechargeStats"/><RechargeView v-else-if="recharge"/><ChatImageView v-else/></template>
