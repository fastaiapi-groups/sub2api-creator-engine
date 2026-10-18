<template><div class="standalone-layout"><slot/></div></template>
<style scoped>
.standalone-layout{min-height:100vh;padding:24px;background:#f3f4f6;transition:background-color .2s ease}
:global(.dark) .standalone-layout{background:#030712}
@media(max-width:640px){.standalone-layout{padding:0}}
</style>
