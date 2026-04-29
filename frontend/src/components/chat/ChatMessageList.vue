<script setup lang="ts">
import type { MessageItem } from '@/api/chat'
import ChatMessageItem from '@/components/chat/ChatMessageItem.vue'

defineProps<{
  messages: MessageItem[]
  showScrollToBottom: boolean
  loadingOlderHistory: boolean
  setMessagesRef: (el: unknown) => void
}>()

const emit = defineEmits<{
  scrollToBottom: []
}>()
</script>

<template>
  <section :ref="setMessagesRef" class="messages-section scroll-area min-w-0 flex-1 overflow-y-auto overflow-x-hidden px-4 py-6">
    <div class="flex min-w-0 flex-col gap-5 max-w-[720px] mx-auto">
      <div
        v-if="loadingOlderHistory"
        class="flex justify-center py-2"
      >
        <svg class="loading-spinner-accent animate-spin w-5 h-5" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
        </svg>
      </div>
      <ChatMessageItem v-for="item in messages" :key="item.id" :item="item" />
    </div>
  </section>

  <Transition name="scroll-bottom-fade">
    <div v-if="showScrollToBottom" class="pointer-events-none absolute right-6 bottom-[132px] z-20">
      <button
        type="button"
        class="scroll-bottom-btn pointer-events-auto w-10 h-10 rounded-full inline-flex items-center justify-center cursor-pointer"
        aria-label="Scroll to bottom"
        @click="emit('scrollToBottom')"
      >
        <span class="scroll-bottom-arrow" aria-hidden="true">↓</span>
      </button>
    </div>
  </Transition>
</template>
