<script setup lang="ts">
import { useRouter } from 'vue-router'

import SettingsPanel from '../components/SettingsPanel.vue'

const router = useRouter()
</script>

<template>
  <div class="settings-page">
    <div class="inner">
      <SettingsPanel @close="router.push('/')" />
    </div>
  </div>
</template>

<style scoped>
.settings-page {
  height: 100vh;
  background: #f2f2f2;
  padding: 24px 12px;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  justify-content: center;
}

.inner {
  width: min(90vw, 1080px);
  height: min(86vh, 760px);
}
</style>
