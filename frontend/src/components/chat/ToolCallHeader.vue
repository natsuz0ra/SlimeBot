<script setup lang="ts">
import MdiIcon from '@/components/ui/MdiIcon.vue'

defineProps<{
  toolIcon: string
  toolLabel: string
  toolSummary: string
  statusLabel: string
  statusDotClass: string
  statusTextClass: string
  status: string
  isAskQuestions: boolean
  qaCount?: number
  qaTitle: string
  collapsed: boolean
}>()

const emit = defineEmits<{
  toggle: []
}>()
</script>

<template>
  <header :class="['tool-header', { 'tool-header--clickable': isAskQuestions }]" @click="emit('toggle')">
    <div class="tool-header-main">
      <div class="tool-meta">
        <MdiIcon :path="toolIcon" :size="16" class="tool-icon flex-shrink-0" />
        <span class="tool-label">{{ toolLabel }}</span>
        <span
          v-if="toolSummary && !isAskQuestions"
          class="tool-summary"
          :title="toolSummary"
        >
          {{ toolSummary }}
        </span>
        <span v-if="isAskQuestions && qaCount !== undefined" class="tool-qa-count">
          {{ qaCount }} {{ qaTitle }}
        </span>
      </div>

      <div class="tool-status ml-auto">
        <span
          class="tool-status-dot"
          :class="[statusDotClass, status === 'executing' ? 'status-dot-pulse' : '']"
        />
        <span :class="statusTextClass">{{ statusLabel }}</span>
        <svg
          v-if="status === 'executing'"
          class="animate-spin-icon w-3.5 h-3.5 flex-shrink-0 tool-status-spinner"
          fill="none"
          viewBox="0 0 24 24"
          aria-hidden="true"
        >
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
        </svg>
        <svg
          v-if="isAskQuestions"
          class="tool-collapse-arrow"
          :class="{ 'tool-collapse-arrow--open': !collapsed }"
          viewBox="0 0 16 16"
          width="14"
          height="14"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M4 6l4 4 4-4" />
        </svg>
      </div>
    </div>
  </header>
</template>
