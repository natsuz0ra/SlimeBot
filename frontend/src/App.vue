<script setup lang="ts">
import { onMounted } from 'vue'
import AppToast from '@/components/ui/AppToast.vue'
import { useTheme } from '@/composables/useTheme'

const { init } = useTheme()
onMounted(() => init())
</script>

<template>
  <router-view />
  <AppToast />
</template>
