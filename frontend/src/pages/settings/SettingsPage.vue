<script setup lang="ts">
import { useRouter } from 'vue-router'

import SettingsPanel from '../../features/settings/components/SettingsPanel.vue'

const router = useRouter()
</script>

<template>
  <div class="settings-page">
    <div class="inner">
      <SettingsPanel @close="router.push('/')" />
    </div>
  </div>
</template>

<style scoped src="./settings-page.css"></style>
