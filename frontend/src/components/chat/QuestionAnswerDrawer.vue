<script setup lang="ts">
import { ref, reactive, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'

interface QuestionItem {
  id: string
  question: string
  options: string[]
  option_descriptions?: string[]
}

interface Answer {
  questionId: string
  selectedOption: number
  customAnswer: string
}

const props = defineProps<{
  visible: boolean
  questions: QuestionItem[]
  toolCallId: string
}>()

const emit = defineEmits<{
  submit: [toolCallId: string, answers: string]
  cancel: [toolCallId: string]
}>()

const { t } = useI18n()
const step = ref<'questions' | 'confirm'>('questions')
const answers = ref<Answer[]>([])
const submitBtnRef = ref<HTMLButtonElement | null>(null)

const tooltipState = reactive({
  visible: false,
  text: '',
  x: 0,
  y: 0,
})

function showTooltip(e: MouseEvent, text: string) {
  const el = e.currentTarget as HTMLElement
  const rect = el.getBoundingClientRect()
  tooltipState.visible = true
  tooltipState.text = text
  tooltipState.x = rect.left + rect.width / 2
  tooltipState.y = rect.top - 8
}

function hideTooltip() {
  tooltipState.visible = false
}

watch(
  () => props.visible,
  async (isOpen) => {
    if (isOpen) {
      step.value = 'questions'
      answers.value = props.questions.map((q) => ({
        questionId: q.id,
        selectedOption: -2,
        customAnswer: '',
      }))
      await nextTick()
      submitBtnRef.value?.focus()
    }
  },
)

function selectOption(qIndex: number, optionIndex: number) {
  const prev = answers.value[qIndex]
  if (!prev) return
  answers.value[qIndex] = { questionId: prev.questionId, selectedOption: optionIndex, customAnswer: '' }
}

function selectCustom(qIndex: number) {
  const prev = answers.value[qIndex]
  if (!prev) return
  answers.value[qIndex] = { questionId: prev.questionId, selectedOption: -1, customAnswer: '' }
}

function updateCustomInput(qIndex: number, value: string) {
  const prev = answers.value[qIndex]
  if (!prev) return
  answers.value[qIndex] = { questionId: prev.questionId, selectedOption: prev.selectedOption, customAnswer: value }
}

function allAnswered(): boolean {
  return answers.value.every((a) => {
    if (!a) return false
    if (a.selectedOption >= 0) return true
    return a.customAnswer.trim().length > 0
  })
}

function getDisplayAnswer(qIndex: number): string {
  const a = answers.value[qIndex]
  if (!a) return ''
  if (a.selectedOption >= 0) return props.questions[qIndex]?.options[a.selectedOption] ?? ''
  return a.customAnswer.trim()
}

function goToConfirm() {
  if (!allAnswered()) return
  step.value = 'confirm'
}

function goBack() {
  step.value = 'questions'
}

function handleSubmit() {
  const answersJSON = JSON.stringify(
    answers.value.map((a) => ({
      questionId: a.questionId,
      selectedOption: a.selectedOption,
      customAnswer: a.customAnswer,
    })),
  )
  emit('submit', props.toolCallId, answersJSON)
}

function handleCancel() {
  emit('cancel', props.toolCallId)
}

function onKeydown(e: KeyboardEvent) {
  if (!props.visible) return
  if (e.key === 'Escape') {
    e.preventDefault()
    if (step.value === 'confirm') {
      goBack()
    } else {
      handleCancel()
    }
  }
}

onMounted(() => document.addEventListener('keydown', onKeydown))
onUnmounted(() => document.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <Transition name="drawer-slide">
      <div v-if="visible" class="drawer-overlay" @click.self="handleCancel">
        <div class="drawer-panel" role="dialog" aria-modal="true" :aria-label="t('qaTitle')">
          <header class="drawer-header">
            <h3 class="drawer-title">{{ t('qaTitle') }}</h3>
          </header>

          <section class="drawer-body sb-scrollbar">
            <!-- Questions step -->
            <template v-if="step === 'questions'">
              <div v-for="(q, qi) in questions" :key="q.id" class="qa-question-block">
                <div class="qa-question-text">
                  <span class="qa-question-index">{{ qi + 1 }}.</span>
                  {{ q.question }}
                </div>
                <div class="qa-options">
                  <label
                    v-for="(opt, oi) in q.options"
                    :key="oi"
                    class="qa-option"
                    :class="{ 'qa-option--selected': answers[qi]?.selectedOption === oi }"
                  >
                    <input
                      type="radio"
                      :name="'q-' + q.id"
                      :checked="answers[qi]?.selectedOption === oi"
                      @change="selectOption(qi, oi)"
                    />
                    <span class="qa-option-radio" />
                    <span class="qa-option-text">{{ opt }}</span>
                    <span v-if="q.option_descriptions?.[oi]" class="qa-option-help ml-auto flex-shrink-0" @mouseenter="showTooltip($event, q.option_descriptions[oi])" @mouseleave="hideTooltip">
                      <span class="qa-option-help-icon">?</span>
                    </span>
                  </label>
                  <!-- Custom input option -->
                  <label
                    class="qa-option"
                    :class="{ 'qa-option--selected': answers[qi]?.selectedOption === -1 }"
                  >
                    <input
                      type="radio"
                      :name="'q-' + q.id"
                      :checked="answers[qi]?.selectedOption === -1"
                      @change="selectCustom(qi)"
                    />
                    <span class="qa-option-radio" />
                    <span class="qa-option-text">{{ t('qaCustomOption') }}</span>
                  </label>
                </div>
                <input
                  v-if="answers[qi]?.selectedOption === -1"
                  type="text"
                  class="qa-custom-input"
                  :placeholder="t('qaCustomPlaceholder')"
                  :value="answers[qi]?.customAnswer"
                  @input="updateCustomInput(qi, ($event.target as HTMLInputElement).value)"
                />
              </div>
            </template>

            <!-- Confirm step -->
            <template v-else>
              <div class="qa-confirm-title">{{ t('qaConfirmTitle') }}</div>
              <div v-for="(q, qi) in questions" :key="q.id" class="qa-confirm-pair">
                <div class="qa-confirm-question">{{ q.question }}</div>
                <div class="qa-confirm-answer">{{ getDisplayAnswer(qi) }}</div>
              </div>
            </template>
          </section>

          <footer class="drawer-footer">
            <template v-if="step === 'questions'">
              <button type="button" class="drawer-btn drawer-btn--cancel" @click="handleCancel">
                {{ t('qaCancel') }}
              </button>
              <button
                ref="submitBtnRef"
                type="button"
                class="drawer-btn drawer-btn--next"
                :disabled="!allAnswered()"
                @click="goToConfirm"
              >
                {{ t('qaNext') }}
              </button>
            </template>
            <template v-else>
              <button type="button" class="drawer-btn drawer-btn--cancel" @click="goBack">
                {{ t('qaBack') }}
              </button>
              <button
                type="button"
                class="drawer-btn drawer-btn--submit"
                @click="handleSubmit"
              >
                {{ t('qaSubmit') }}
              </button>
            </template>
          </footer>
        </div>
      </div>
    </Transition>
    <Transition name="tooltip-fade">
      <div
        v-if="tooltipState.visible"
        class="qa-desc-floating-tooltip"
        :style="{ left: tooltipState.x + 'px', top: tooltipState.y + 'px' }"
      >
        {{ tooltipState.text }}
        <span class="qa-desc-floating-arrow" />
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
  max-height: 80vh;
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
  background: rgba(255, 255, 255, 0.96);
}

.dark .drawer-panel {
  background: rgba(24, 24, 48, 0.96);
  box-shadow:
    0 -8px 40px rgba(0, 0, 0, 0.5),
    0 0 0 1px rgba(255, 255, 255, 0.06),
    inset 0 1px 0 rgba(255, 255, 255, 0.06);
}

.drawer-header {
  padding: 16px 20px 12px;
  border-bottom: 1px solid var(--tool-section-border);
  display: flex;
  align-items: center;
  gap: 10px;
}

.drawer-title {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: 0.01em;
}

.drawer-step-badge {
  font-size: 13px;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 6px;
  background: var(--primary-alpha-08);
  color: var(--text-secondary);
}

.drawer-body {
  flex: 1;
  overflow-y: auto;
  padding: 14px 20px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.qa-question-block {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.qa-question-text {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  line-height: 1.5;
}

.qa-question-index {
  color: var(--text-secondary);
  margin-right: 4px;
}

.qa-options {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-left: 8px;
}

.qa-option {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-radius: 8px;
  cursor: pointer;
  transition: background-color 150ms ease;
}

.qa-option:hover {
  background: var(--tool-section-bg);
}

.qa-option input[type='radio'] {
  display: none;
}

.qa-option-radio {
  width: 16px;
  height: 16px;
  border-radius: 50%;
  border: 2px solid var(--tool-section-border);
  flex-shrink: 0;
  position: relative;
  transition: border-color 150ms ease;
}

.qa-option--selected .qa-option-radio {
  border-color: var(--tool-running-dot);
}

.qa-option--selected .qa-option-radio::after {
  content: '';
  position: absolute;
  inset: 2px;
  border-radius: 50%;
  background: var(--tool-running-dot);
}

.qa-option-text {
  font-size: 13px;
  color: var(--text-primary);
}

.qa-option-help {
  display: inline-flex;
  align-items: center;
  cursor: help;
}

.qa-option-help-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  font-size: 10px;
  font-weight: 700;
  background: var(--tool-section-border);
  color: var(--text-secondary);
  line-height: 1;
  cursor: help;
}

.qa-custom-input {
  margin-left: 8px;
  width: calc(100% - 8px);
  border: 1px solid var(--tool-section-border);
  border-radius: 8px;
  padding: 8px 10px;
  background: var(--tool-section-bg);
  color: var(--text-primary);
  font-size: 13px;
  outline: none;
  font-family: inherit;
}

.qa-custom-input:focus {
  border-color: var(--primary-alpha-08);
}

.qa-confirm-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: 4px;
}

.qa-confirm-pair {
  border: 1px solid var(--tool-section-border);
  border-radius: 10px;
  padding: 10px;
  background: var(--tool-section-bg);
}

.qa-confirm-question {
  font-size: 14px;
  color: var(--text-secondary);
  margin-bottom: 4px;
}

.qa-confirm-answer {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
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
  transition:
    background-color 180ms ease,
    color 180ms ease,
    box-shadow 180ms ease,
    border-color 180ms ease;
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

.drawer-btn--next,
.drawer-btn--submit {
  flex: 1;
  background: var(--tool-success-bg);
  border: 1px solid var(--tool-success-border);
  color: var(--tool-success-text);
}

.drawer-btn--next:hover,
.drawer-btn--submit:hover {
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

<style>
.qa-desc-floating-tooltip {
  position: fixed;
  transform: translate(-50%, -100%);
  width: 220px;
  border-radius: 8px;
  padding: 8px 12px;
  font-size: 12px;
  line-height: 20px;
  color: #fff;
  background: rgba(0, 0, 0, 0.78);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  z-index: 99999;
  pointer-events: none;
}

.qa-desc-floating-arrow {
  position: absolute;
  bottom: -4px;
  left: 50%;
  transform: translateX(-50%) rotate(45deg);
  width: 8px;
  height: 8px;
  background: rgba(0, 0, 0, 0.78);
}

.tooltip-fade-enter-active,
.tooltip-fade-leave-active {
  transition: opacity 150ms;
}

.tooltip-fade-enter-from,
.tooltip-fade-leave-to {
  opacity: 0;
}
</style>
