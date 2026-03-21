<script setup lang="ts">
import type { MessageItem } from '@/api/chat'
import ChatMessageItem from '@/components/chat/ChatMessageItem.vue'

defineProps<{
  messages: MessageItem[]
  showScrollToBottom: boolean
  setMessagesRef: (el: unknown) => void
}>()

const emit = defineEmits<{
  scrollToBottom: []
}>()
</script>

<template>
  <section :ref="setMessagesRef" class="messages-section scroll-area flex-1 overflow-y-auto px-4 py-6">
    <div class="flex flex-col gap-5 max-w-[720px] mx-auto">
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
