<template>
  <div class="flex items-center gap-1.5 py-1">
    <span class="typing-dot" />
    <span class="typing-dot" style="animation-delay: 0.16s" />
    <span class="typing-dot" style="animation-delay: 0.32s" />
  </div>
</template>

<style scoped>
.typing-dot {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 9999px;
  background: var(--sb-brand);
  animation: bounce-dot 1.1s cubic-bezier(0.455, 0.03, 0.515, 0.955) infinite;
}

@media (prefers-reduced-motion: reduce) {
  .typing-dot {
    animation: none;
    opacity: 0.6;
  }
}
</style>
