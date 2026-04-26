<script setup lang="ts">
import { ref, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  visible: boolean
  planContent: string
}>()

const emit = defineEmits<{
  execute: []
  modify: [feedback: string]
  cancel: []
}>()

const { t } = useI18n()
const executeBtnRef = ref<HTMLButtonElement | null>(null)
const mode = ref<'options' | 'modify'>('options')
const feedback = ref('')
const textareaRef = ref<HTMLTextAreaElement | null>(null)

watch(
  () => props.visible,
  async (isOpen) => {
    if (isOpen) {
      mode.value = 'options'
      feedback.value = ''
      await nextTick()
      executeBtnRef.value?.focus()
    }
  },
)

watch(mode, async (m) => {
  if (m === 'modify') {
    await nextTick()
    textareaRef.value?.focus()
  }
})

function onKeydown(e: KeyboardEvent) {
  if (!props.visible) return
  if (mode.value === 'modify') return
  if (e.key === 'Enter') {
    e.preventDefault()
    emit('execute')
  } else if (e.key === 'Escape') {
    e.preventDefault()
    emit('cancel')
  }
}

onMounted(() => document.addEventListener('keydown', onKeydown))
onUnmounted(() => document.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <Transition name="drawer-slide">
      <div v-if="visible" class="drawer-overlay" @click.self="emit('cancel')">
        <div class="drawer-panel" role="dialog" aria-modal="true" :aria-label="t('planConfirmTitle')">
          <header class="drawer-header">
            <h3 class="drawer-title">{{ t('planConfirmTitle') }}</h3>
          </header>

          <section class="drawer-body">
            <div v-if="mode === 'options'" class="drawer-plan-preview">
              <pre class="drawer-plan-content">{{ planContent }}</pre>
            </div>
            <div v-else class="drawer-modify">
              <textarea
                ref="textareaRef"
                v-model="feedback"
                class="drawer-feedback-input"
                :placeholder="t('planConfirmModifyPlaceholder')"
                rows="3"
                @keydown.escape="mode = 'options'"
              />
            </div>
          </section>

          <footer class="drawer-footer">
            <template v-if="mode === 'options'">
              <button
                type="button"
                class="drawer-btn drawer-btn--cancel"
                @click="emit('cancel')"
              >
                {{ t('planConfirmCancel') }}
              </button>
              <button
                type="button"
                class="drawer-btn drawer-btn--modify"
                @click="mode = 'modify'"
              >
                {{ t('planConfirmModify') }}
              </button>
              <button
                ref="executeBtnRef"
                type="button"
                class="drawer-btn drawer-btn--execute"
                @click="emit('execute')"
              >
                {{ t('planConfirmExecute') }}
              </button>
            </template>
            <template v-else>
              <button
                type="button"
                class="drawer-btn drawer-btn--cancel"
                @click="mode = 'options'"
              >
                {{ t('planConfirmBack') }}
              </button>
              <button
                type="button"
                class="drawer-btn drawer-btn--execute"
                :disabled="!feedback.trim()"
                @click="emit('modify', feedback.trim())"
              >
                {{ t('planConfirmSendFeedback') }}
              </button>
            </template>
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.drawer-overlay {
  position: fixed;
  inset: 0;
  z-index: 300;
  display: flex;
  align-items: flex-end;
  justify-content: center;
  background: rgba(0, 0, 0, 0.18);
  backdrop-filter: blur(2px);
}

.drawer-panel {
  width: 100%;
  max-width: 520px;
  max-height: 70vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-radius: 20px 20px 0 0;
  background: var(--bg-main);
  border: 1px solid var(--tool-card-border);
  border-bottom: none;
  box-shadow:
    0 -8px 40px rgba(0, 0, 0, 0.2),
    0 0 0 1px var(--primary-alpha-08),
    inset 0 1px 0 rgba(255, 255, 255, 0.6);
  backdrop-filter: blur(20px) saturate(1.4);
}

:root:not(.dark) .drawer-panel {
  background: rgba(255, 255, 255, 0.82);
}

.dark .drawer-panel {
  background: rgba(24, 24, 48, 0.88);
  box-shadow:
    0 -8px 40px rgba(0, 0, 0, 0.5),
    0 0 0 1px rgba(255, 255, 255, 0.06),
    inset 0 1px 0 rgba(255, 255, 255, 0.06);
}

.drawer-header {
  padding: 16px 20px 12px;
  border-bottom: 1px solid var(--tool-section-border);
}

.drawer-title {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: 0.01em;
}

.drawer-body {
  flex: 1;
  overflow-y: auto;
  padding: 14px 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.drawer-plan-preview {
  border: 1px solid var(--tool-section-border);
  border-radius: 10px;
  padding: 10px;
  background: var(--tool-section-bg);
  max-height: 300px;
  overflow-y: auto;
}

.drawer-plan-content {
  margin: 0;
  font-size: 14px;
  line-height: 1.5;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  color: var(--text-primary);
  font-family: var(--font-mono);
}

.drawer-feedback-input {
  width: 100%;
  border: 1px solid var(--tool-section-border);
  border-radius: 10px;
  padding: 10px;
  background: var(--tool-section-bg);
  color: var(--text-primary);
  font-size: 14px;
  line-height: 1.5;
  resize: none;
  outline: none;
  font-family: inherit;
}

.drawer-feedback-input:focus {
  border-color: var(--primary-alpha-08);
}

.drawer-footer {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 14px 20px 18px;
  border-top: 1px solid var(--tool-section-border);
}

.drawer-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 36px;
  padding: 8px 18px;
  border-radius: 10px;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  transition: background-color 180ms ease, color 180ms ease, box-shadow 180ms ease, border-color 180ms ease;
}

.drawer-btn:focus-visible {
  outline: 2px solid var(--focus-ring);
  outline-offset: 2px;
}

.drawer-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.drawer-btn--cancel {
  background: var(--tool-section-bg);
  border: 1px solid var(--tool-section-border);
  color: var(--text-primary);
}

.drawer-btn--cancel:hover {
  background: var(--tool-error-bg);
  border-color: var(--tool-error-border);
}

.drawer-btn--modify {
  background: rgba(234, 179, 8, 0.1);
  border: 1px solid rgba(234, 179, 8, 0.3);
  color: #a16207;
}

.dark .drawer-btn--modify {
  color: #facc15;
}

.drawer-btn--modify:hover {
  background: rgba(234, 179, 8, 0.2);
}

.drawer-btn--execute {
  flex: 1;
  background: var(--tool-success-bg);
  border: 1px solid var(--tool-success-border);
  color: var(--tool-success-text);
}

.drawer-btn--execute:hover {
  background: var(--tool-success-bg-hover);
  box-shadow: 0 2px 8px rgba(16, 185, 129, 0.22);
}

/* Slide-up / slide-down transition */
.drawer-slide-enter-active,
.drawer-slide-leave-active {
  transition: opacity 300ms ease-out;
}

.drawer-slide-enter-active .drawer-panel,
.drawer-slide-leave-active .drawer-panel {
  transition: transform 300ms cubic-bezier(0.16, 1, 0.3, 1);
}

.drawer-slide-enter-from,
.drawer-slide-leave-to {
  opacity: 0;
}

.drawer-slide-enter-from .drawer-panel,
.drawer-slide-leave-to .drawer-panel {
  transform: translateY(100%);
}

@media (max-width: 640px) {
  .drawer-panel {
    max-width: 100%;
    border-radius: 16px 16px 0 0;
  }

  .drawer-footer {
    padding: 12px 16px 16px;
  }

  .drawer-body {
    padding: 12px 16px;
  }

  .drawer-header {
    padding: 14px 16px 10px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .drawer-slide-enter-active,
  .drawer-slide-leave-active,
  .drawer-slide-enter-active .drawer-panel,
  .drawer-slide-leave-active .drawer-panel {
    transition: none;
  }
}
</style>
