<script setup lang="ts">
withDefaults(defineProps<{
  size?: number
  white?: boolean
}>(), {
  size: 24,
  white: false,
})
</script>

<template>
  <img
    src="/slime-icon.svg"
    :width="size"
    :height="size"
    alt="Corner"
    class="block object-contain"
    :style="white ? 'filter: brightness(0) invert(1)' : ''"
  />
</template>
