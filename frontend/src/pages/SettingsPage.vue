<script setup lang="ts">
import { useRouter } from 'vue-router'
import SettingsPanel from '@/components/settings/SettingsPanel.vue'

const router = useRouter()
</script>

<template>
  <div class="h-screen bg-gray-50 flex items-center justify-center p-4 sm:p-6">
    <div class="w-full bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden" style="max-width: min(90vw, 1080px); height: min(86vh, 760px)">
      <SettingsPanel @close="router.push('/')" />
    </div>
  </div>
</template>
