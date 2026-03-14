<script setup lang="ts">
import { useRouter } from 'vue-router'
import SettingsPanel from '@/components/settings/SettingsPanel.vue'

const router = useRouter()
</script>

<template>
  <div class="h-screen flex items-center justify-center p-4 sm:p-6 settings-page">
    <div class="w-full rounded-xl overflow-hidden settings-shell" style="max-width: min(90vw, 1080px); height: min(86vh, 760px)">
      <SettingsPanel @close="router.push('/')" />
    </div>
  </div>
</template>

<style scoped>
.settings-page {
  background: var(--bg-base);
}

.settings-shell {
  background: var(--bg-main);
  border: 1px solid var(--card-border);
  box-shadow: var(--page-card-shadow);
}
</style>
