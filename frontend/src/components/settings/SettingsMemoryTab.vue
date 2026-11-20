<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { mdiDeleteOutline } from '@mdi/js'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import AppTextInput from '@/components/ui/AppTextInput.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'
import type { MemorySnapshot, MemoryTarget, MemoryTargetState } from '@/types/settings'

const props = defineProps<{
  memoryEnabled: boolean
  memoryUserProfileEnabled: boolean
  memoryCharLimit: number
  memoryUserCharLimit: number
  memoryNudgeInterval: number
  memorySnapshot: MemorySnapshot | null
}>()

const emit = defineEmits<{
  memoryEnabledChange: [value: boolean]
  memoryUserProfileEnabledChange: [value: boolean]
  memoryCharLimitChange: [value: number]
  memoryUserCharLimitChange: [value: number]
  memoryNudgeIntervalChange: [value: number]
  deleteEntry: [target: MemoryTarget, index: number]
  clearTarget: [target: MemoryTarget | 'all']
}>()

const { t } = useI18n()
const memoryCharLimitDraft = ref(String(props.memoryCharLimit))
const memoryUserCharLimitDraft = ref(String(props.memoryUserCharLimit))
const memoryNudgeIntervalDraft = ref(String(props.memoryNudgeInterval))
const memoryCharLimitLastCommitted = ref(props.memoryCharLimit)
const memoryUserCharLimitLastCommitted = ref(props.memoryUserCharLimit)
const memoryNudgeIntervalLastCommitted = ref(props.memoryNudgeInterval)
const tooltipState = reactive({
  visible: false,
  text: '',
  x: 0,
  y: 0,
})
const memoryStates = computed(() => {
  const states: MemoryTargetState[] = []
  if (props.memorySnapshot?.memory) states.push(props.memorySnapshot.memory)
  if (props.memorySnapshot?.user) states.push(props.memorySnapshot.user)
  return states
})

const memorySummary = computed(() => [
  createSummary('memory', props.memorySnapshot?.memory, props.memoryCharLimit, props.memoryEnabled),
  createSummary('user', props.memorySnapshot?.user, props.memoryUserCharLimit, props.memoryUserProfileEnabled),
])

const totalEntryCount = computed(() => memorySummary.value.reduce((total, state) => total + state.entryCount, 0))

function createSummary(target: MemoryTarget, state: MemoryTargetState | undefined, fallbackLimit: number, fallbackEnabled: boolean) {
  const charLimit = state?.charLimit || fallbackLimit
  const usageChars = state?.usageChars || 0
  return {
    target,
    title: targetTitle(target),
    enabled: state?.enabled ?? fallbackEnabled,
    usageChars,
    charLimit,
    entryCount: state?.entryCount ?? state?.entries.length ?? 0,
    percent: percent({ usageChars, charLimit } as MemoryTargetState),
  }
}

function targetTitle(target: MemoryTarget) {
  return target === 'memory' ? t('memoryPersonalNotes') : t('memoryUserProfile')
}

function percent(state?: MemoryTargetState) {
  if (!state || state.charLimit <= 0) return 0
  return Math.min(100, Math.round((state.usageChars / state.charLimit) * 100))
}

function sanitizeNumericInput(value: string) {
  return value.replace(/\D/g, '')
}

function clampNumber(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value))
}

function commitNumberDraft(draft: typeof memoryCharLimitDraft, lastCommitted: typeof memoryCharLimitLastCommitted, fallback: number, min: number, max: number, emitChange: (value: number) => void) {
  const parsed = Number(draft.value)
  const nextValue = Number.isFinite(parsed) && draft.value !== '' ? clampNumber(parsed, min, max) : fallback
  draft.value = String(nextValue)
  if (nextValue !== fallback && nextValue !== lastCommitted.value) {
    lastCommitted.value = nextValue
    emitChange(nextValue)
  }
}

function updateMemoryCharLimitDraft(value: string) {
  memoryCharLimitDraft.value = sanitizeNumericInput(value)
}

function updateMemoryUserCharLimitDraft(value: string) {
  memoryUserCharLimitDraft.value = sanitizeNumericInput(value)
}

function updateMemoryNudgeIntervalDraft(value: string) {
  memoryNudgeIntervalDraft.value = sanitizeNumericInput(value)
}

function commitMemoryCharLimitDraft() {
  commitNumberDraft(memoryCharLimitDraft, memoryCharLimitLastCommitted, props.memoryCharLimit, 200, 20000, (value) => emit('memoryCharLimitChange', value))
}

function commitMemoryUserCharLimitDraft() {
  commitNumberDraft(memoryUserCharLimitDraft, memoryUserCharLimitLastCommitted, props.memoryUserCharLimit, 200, 20000, (value) => emit('memoryUserCharLimitChange', value))
}

function commitMemoryNudgeIntervalDraft() {
  commitNumberDraft(memoryNudgeIntervalDraft, memoryNudgeIntervalLastCommitted, props.memoryNudgeInterval, 1, 100, (value) => emit('memoryNudgeIntervalChange', value))
}

function showMemoryTooltip(event: MouseEvent | FocusEvent, text: string) {
  const el = event.currentTarget as HTMLElement
  const rect = el.getBoundingClientRect()
  const tooltipWidth = Math.min(260, window.innerWidth - 32)
  const centeredX = rect.left + rect.width / 2
  tooltipState.visible = true
  tooltipState.text = text
  tooltipState.x = Math.min(Math.max(centeredX, 16 + tooltipWidth / 2), window.innerWidth - 16 - tooltipWidth / 2)
  tooltipState.y = rect.top - 8
}

function hideMemoryTooltip() {
  tooltipState.visible = false
}

watch(() => props.memoryCharLimit, (value) => {
  memoryCharLimitDraft.value = String(value)
  memoryCharLimitLastCommitted.value = value
})

watch(() => props.memoryUserCharLimit, (value) => {
  memoryUserCharLimitDraft.value = String(value)
  memoryUserCharLimitLastCommitted.value = value
})

watch(() => props.memoryNudgeInterval, (value) => {
  memoryNudgeIntervalDraft.value = String(value)
  memoryNudgeIntervalLastCommitted.value = value
})

onMounted(() => {
  window.addEventListener('scroll', hideMemoryTooltip, true)
  window.addEventListener('resize', hideMemoryTooltip)
})

onUnmounted(() => {
  window.removeEventListener('scroll', hideMemoryTooltip, true)
  window.removeEventListener('resize', hideMemoryTooltip)
})
</script>

<template>
  <div class="memory-settings-layout">
    <div class="memory-page-head">
      <div>
        <p class="settings-item-sub">{{ t('memoryContextNotice') }}</p>
      </div>
      <button
        type="button"
        class="memory-danger-btn cursor-pointer"
        @click="emit('clearTarget', 'all')"
      >
        {{ t('memoryClearAll') }}
      </button>
    </div>

    <section class="settings-card memory-overview-card">
      <div class="memory-toggle-grid">
        <div class="memory-toggle-row">
          <div>
            <div class="memory-label-with-help">
              <span class="settings-field-label">{{ t('memoryLongTerm') }}</span>
              <button
                type="button"
                class="memory-help-trigger"
                :aria-label="t('memoryLongTermHelp')"
                @mouseenter="showMemoryTooltip($event, t('memoryLongTermHelp'))"
                @mouseleave="hideMemoryTooltip"
                @focus="showMemoryTooltip($event, t('memoryLongTermHelp'))"
                @blur="hideMemoryTooltip"
              >
                ?
              </button>
            </div>
          </div>
          <ToggleSwitch :model-value="memoryEnabled" @update:model-value="emit('memoryEnabledChange', $event)" />
        </div>
        <div class="memory-toggle-row">
          <div>
            <div class="memory-label-with-help">
              <span class="settings-field-label">{{ t('memoryUserProfileToggle') }}</span>
              <button
                type="button"
                class="memory-help-trigger"
                :aria-label="t('memoryUserProfileHelp')"
                @mouseenter="showMemoryTooltip($event, t('memoryUserProfileHelp'))"
                @mouseleave="hideMemoryTooltip"
                @focus="showMemoryTooltip($event, t('memoryUserProfileHelp'))"
                @blur="hideMemoryTooltip"
              >
                ?
              </button>
            </div>
          </div>
          <ToggleSwitch :model-value="memoryUserProfileEnabled" @update:model-value="emit('memoryUserProfileEnabledChange', $event)" />
        </div>
      </div>

      <div class="memory-summary-grid">
        <article v-for="state in memorySummary" :key="state.target" class="memory-summary-card">
          <div class="memory-summary-topline">
            <span class="settings-item-name">{{ state.title }}</span>
            <span class="memory-percent">{{ state.percent }}%</span>
          </div>
          <div class="memory-progress-track" aria-hidden="true">
            <span class="memory-progress-fill" :style="{ width: `${state.percent}%` }" />
          </div>
          <div class="memory-summary-meta">
            <span>{{ state.usageChars }} / {{ state.charLimit }} {{ t('memoryCharsUnit') }}</span>
            <span>{{ state.entryCount }} {{ t('memoryEntriesUnit') }}</span>
          </div>
        </article>
      </div>
    </section>

    <section class="settings-card memory-config-card">
      <div class="memory-card-heading">
        <div>
          <div class="settings-field-label">{{ t('memoryPolicyTitle') }}</div>
          <div class="settings-item-sub">{{ t('memoryPolicyDesc') }}</div>
        </div>
      </div>
      <div class="memory-config-grid">
        <label class="memory-config-field">
          <span class="memory-label-with-help">
            <span class="settings-dialog-label">{{ t('memoryCharLimit') }}</span>
            <button
              type="button"
              class="memory-help-trigger"
              :aria-label="t('memoryCharLimitHelp')"
              @mouseenter="showMemoryTooltip($event, t('memoryCharLimitHelp'))"
              @mouseleave="hideMemoryTooltip"
              @focus="showMemoryTooltip($event, t('memoryCharLimitHelp'))"
              @blur="hideMemoryTooltip"
            >
              ?
            </button>
          </span>
          <AppTextInput
            :model-value="memoryCharLimitDraft"
            class="memory-number-input"
            type="text"
            inputmode="numeric"
            pattern="[0-9]*"
            autocomplete="off"
            @update:model-value="updateMemoryCharLimitDraft"
            @change="commitMemoryCharLimitDraft"
            @blur="commitMemoryCharLimitDraft"
          />
          <span class="memory-field-hint">{{ t('memoryCharsUnit') }}</span>
        </label>
        <label class="memory-config-field">
          <span class="memory-label-with-help">
            <span class="settings-dialog-label">{{ t('memoryUserCharLimit') }}</span>
            <button
              type="button"
              class="memory-help-trigger"
              :aria-label="t('memoryUserCharLimitHelp')"
              @mouseenter="showMemoryTooltip($event, t('memoryUserCharLimitHelp'))"
              @mouseleave="hideMemoryTooltip"
              @focus="showMemoryTooltip($event, t('memoryUserCharLimitHelp'))"
              @blur="hideMemoryTooltip"
            >
              ?
            </button>
          </span>
          <AppTextInput
            :model-value="memoryUserCharLimitDraft"
            class="memory-number-input"
            type="text"
            inputmode="numeric"
            pattern="[0-9]*"
            autocomplete="off"
            @update:model-value="updateMemoryUserCharLimitDraft"
            @change="commitMemoryUserCharLimitDraft"
            @blur="commitMemoryUserCharLimitDraft"
          />
          <span class="memory-field-hint">{{ t('memoryCharsUnit') }}</span>
        </label>
        <label class="memory-config-field">
          <span class="memory-label-with-help">
            <span class="settings-dialog-label">{{ t('memoryNudgeInterval') }}</span>
            <button
              type="button"
              class="memory-help-trigger"
              :aria-label="t('memoryNudgeIntervalHelp')"
              @mouseenter="showMemoryTooltip($event, t('memoryNudgeIntervalHelp'))"
              @mouseleave="hideMemoryTooltip"
              @focus="showMemoryTooltip($event, t('memoryNudgeIntervalHelp'))"
              @blur="hideMemoryTooltip"
            >
              ?
            </button>
          </span>
          <AppTextInput
            :model-value="memoryNudgeIntervalDraft"
            class="memory-number-input"
            type="text"
            inputmode="numeric"
            pattern="[0-9]*"
            autocomplete="off"
            @update:model-value="updateMemoryNudgeIntervalDraft"
            @change="commitMemoryNudgeIntervalDraft"
            @blur="commitMemoryNudgeIntervalDraft"
          />
          <span class="memory-field-hint">{{ t('memoryTurnsUnit') }}</span>
        </label>
      </div>
    </section>

    <section class="memory-content-section">
      <div class="memory-card-heading">
        <div>
          <div class="memory-label-with-help">
            <span class="settings-field-label">{{ t('memoryContentTitle') }}</span>
            <button
              type="button"
              class="memory-help-trigger"
              :aria-label="t('memoryContentHelp')"
              @mouseenter="showMemoryTooltip($event, t('memoryContentHelp'))"
              @mouseleave="hideMemoryTooltip"
              @focus="showMemoryTooltip($event, t('memoryContentHelp'))"
              @blur="hideMemoryTooltip"
            >
              ?
            </button>
          </div>
          <div class="settings-item-sub">{{ totalEntryCount }} {{ t('memoryEntriesUnit') }}</div>
        </div>
      </div>

      <div class="memory-library-grid">
        <article
          v-for="state in memoryStates"
          :key="state.target"
          class="settings-card memory-library-card"
        >
          <div class="memory-library-header">
            <div class="memory-library-title-wrap">
              <div class="settings-item-name">{{ targetTitle(state.target) }}</div>
              <div class="memory-library-meta">
                <span class="memory-status-pill" :class="state.enabled ? 'memory-status-on' : 'memory-status-off'">
                  {{ state.enabled ? t('memoryEnabledStatus') : t('memoryDisabledStatus') }}
                </span>
                <span>{{ state.entryCount }} {{ t('memoryEntriesUnit') }}</span>
              </div>
            </div>
            <button
              type="button"
              class="account-edit-btn memory-clear-btn cursor-pointer"
              @click="emit('clearTarget', state.target)"
            >
              {{ t('memoryClearTarget') }}
            </button>
          </div>

          <div class="memory-library-usage">
            <div class="memory-summary-topline">
              <span>{{ state.usageChars }} / {{ state.charLimit }} {{ t('memoryCharsUnit') }}</span>
              <span>{{ percent(state) }}%</span>
            </div>
            <div class="memory-progress-track" aria-hidden="true">
              <span class="memory-progress-fill" :style="{ width: `${percent(state)}%` }" />
            </div>
          </div>

          <div v-if="state.entries.length === 0" class="memory-empty-state">
            {{ t('memoryEmpty') }}
          </div>
          <div v-else class="memory-entry-list">
            <div v-for="(entry, index) in state.entries" :key="`${state.target}-${index}`" class="memory-entry-row">
              <p class="settings-item-sub memory-entry-text">{{ entry }}</p>
              <button
                type="button"
                class="delete-btn memory-entry-delete cursor-pointer"
                :aria-label="`${t('delete')} ${targetTitle(state.target)} ${index + 1}`"
                @click="emit('deleteEntry', state.target, index)"
              >
                <MdiIcon :path="mdiDeleteOutline" :size="15" />
              </button>
            </div>
          </div>
        </article>
      </div>

      <div v-if="memoryStates.length === 0" class="memory-empty-state">
        {{ t('memoryEmpty') }}
      </div>
    </section>

    <Teleport to="body">
      <Transition name="memory-tooltip-fade">
        <div
          v-if="tooltipState.visible"
          class="memory-help-tooltip"
          role="tooltip"
          :style="{ left: `${tooltipState.x}px`, top: `${tooltipState.y}px` }"
        >
          {{ tooltipState.text }}
        </div>
      </Transition>
    </Teleport>
  </div>
</template>
