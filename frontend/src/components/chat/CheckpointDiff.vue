<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ diff: string }>()
const { t } = useI18n()

const lines = computed(() => {
  let inHunk = false
  return props.diff.replace(/\r\n/g, '\n').replace(/\n$/, '').split('\n').map((text) => {
    if (text.startsWith('diff --git ')) inHunk = false
    if (text.startsWith('@@')) {
      inHunk = true
      return { text, kind: 'hunk' }
    }
    if (inHunk && text.startsWith('+')) return { text, kind: 'added' }
    if (inHunk && text.startsWith('-')) return { text, kind: 'deleted' }
    return { text, kind: inHunk && text.startsWith(' ') ? 'context' : 'meta' }
  })
})
</script>

<template>
  <pre class="checkpoint-diff sb-scrollbar" role="region" :aria-label="t('checkpoints.diff')" tabindex="0"><code><span v-for="(line, index) in lines" :key="index" class="diff-line" :class="`diff-${line.kind}`">{{ line.text + '\n' }}</span></code></pre>
</template>

<style scoped>
.checkpoint-diff {
  max-height: clamp(11rem, 31dvh, 22rem);
  overflow: auto;
  margin: 0;
  padding-block: 0.25rem;
  border: 1px solid var(--tool-section-border);
  border-radius: 0.6rem;
  background: var(--tool-section-bg);
  color: var(--text-primary);
  cursor: text;
  font-family: var(--font-mono);
  font-size: 0.75rem;
  line-height: 1.65;
  tab-size: 4;
  white-space: pre;
}

.checkpoint-diff:focus-visible {
  outline: 2px solid var(--focus-ring);
  outline-offset: 2px;
}

.checkpoint-diff code {
  display: grid;
  min-width: max-content;
  font-family: inherit;
}

.diff-line { padding-inline: 0.75rem; }
.diff-meta { color: var(--text-secondary); }
.diff-hunk { background: var(--tool-running-bg); color: var(--tool-running-text); }
.diff-added { background: var(--tool-success-bg); color: var(--tool-success-text); }
.diff-deleted { background: var(--tool-error-bg); color: var(--tool-error-text); }
</style>
