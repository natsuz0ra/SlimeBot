<script setup lang="ts">
import { LoaderCircle } from 'lucide-vue-next'

withDefaults(
  defineProps<{
    sizeClass?: string
  }>(),
  { sizeClass: 'w-5 h-5' },
)
</script>

<template>
  <LoaderCircle :class="['animate-spin', sizeClass]" class="loading-spinner-accent" :stroke-width="1.75" aria-hidden="true" />
</template>

<style scoped>
.loading-spinner-accent {
  color: var(--sb-brand);
}
</style>
