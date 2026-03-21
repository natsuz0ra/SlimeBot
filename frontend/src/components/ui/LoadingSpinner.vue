<script setup lang="ts">
withDefaults(
  defineProps<{
    sizeClass?: string
  }>(),
  { sizeClass: 'w-5 h-5' },
)
</script>

<template>
  <svg :class="['animate-spin', sizeClass]" class="loading-spinner-accent" fill="none" viewBox="0 0 24 24">
    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
  </svg>
</template>

<style scoped>
.loading-spinner-accent {
  color: var(--sb-brand);
}
</style>
