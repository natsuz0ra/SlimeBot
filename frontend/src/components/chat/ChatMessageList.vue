<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { MessageItem } from '@/api/chat'
import ChatMessageItem from '@/components/chat/ChatMessageItem.vue'

defineProps<{
  messages: MessageItem[]
  focusedMessageId?: string
  hasNewerHistory?: boolean
  loadingNewerMessages?: boolean
  showScrollToBottom: boolean
  loadingOlderHistory: boolean
  setMessagesRef: (el: unknown) => void
}>()

const { t } = useI18n()
const emit = defineEmits<{
  scrollToBottom: []
  loadNewer: []
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
      <div v-for="item in messages" :key="item.id" :data-message-id="item.id" :class="{ 'search-message-focus': item.id === focusedMessageId }" class="message-anchor min-w-0">
        <ChatMessageItem :item="item" />
      </div>
      <button v-if="hasNewerHistory" type="button" class="search-latest-link" :disabled="loadingNewerMessages" @click="emit('loadNewer')">{{ t('chatSearchNewer') }}</button>
      <button v-if="hasNewerHistory" type="button" class="search-latest-link" :disabled="loadingNewerMessages" @click="emit('scrollToBottom')">{{ t('chatSearchLatest') }} ↓</button>
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

<style scoped>
.message-anchor { scroll-margin-block: 24px; border-radius: 14px; }
.search-message-focus { outline: 2px solid var(--sb-brand); outline-offset: 7px; background: var(--primary-alpha-05); }
.search-latest-link { align-self: center; color: var(--sb-brand); font-size: 13px; padding: 8px 16px; border-radius: 999px; background: var(--primary-alpha-10); cursor: pointer; }
.search-latest-link:focus-visible { outline: 2px solid var(--sb-brand); outline-offset: 3px; }
</style>
