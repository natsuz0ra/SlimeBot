<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed, ref, watch } from 'vue'
import type { MessageItem } from '@/api/chat'
import ChatHistoryNav from '@/components/chat/ChatHistoryNav.vue'
import ChatMessageItem from '@/components/chat/ChatMessageItem.vue'

const props = defineProps<{
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
  jumpToMessage: [target: HTMLElement]
}>()

const scroller = ref<HTMLElement | null>(null)
const content = ref<HTMLElement | null>(null)
const history = computed(() => props.messages.filter(message => message.role === 'user'))
const activeId = ref<string | null>(null)
let anchors: HTMLElement[] = []

function setScroller(el: unknown) {
  scroller.value = el instanceof HTMLElement ? el : null
  props.setMessagesRef(el)
}

function updateActiveMessage() {
  const el = scroller.value
  if (!el || !anchors.length) {
    activeId.value = null
    return
  }
  if (el.scrollHeight - el.clientHeight - el.scrollTop <= 2) {
    activeId.value = anchors[anchors.length - 1]?.dataset.historyMessage ?? null
    return
  }

  const readingLine = el.getBoundingClientRect().top + 36
  let first = 0
  let last = anchors.length
  while (first < last) {
    const middle = Math.floor((first + last) / 2)
    if (anchors[middle]!.getBoundingClientRect().top <= readingLine) first = middle + 1
    else last = middle
  }
  activeId.value = anchors[Math.max(0, first - 1)]?.dataset.historyMessage ?? null
}

function jumpToMessage(id: string) {
  const target = anchors.find(anchor => anchor.dataset.historyMessage === id)
  if (target) emit('jumpToMessage', target)
}

watch([scroller, content, history], ([el, body], _, onCleanup) => {
  anchors = body ? Array.from(body.querySelectorAll<HTMLElement>('[data-history-message]')) : []
  updateActiveMessage()
  if (!el || !body || !anchors.length) return
  let frame = 0
  const schedule = () => {
    if (frame) return
    frame = requestAnimationFrame(() => {
      frame = 0
      updateActiveMessage()
    })
  }
  el.addEventListener('scroll', schedule, { passive: true })
  const observer = new ResizeObserver(schedule)
  observer.observe(el)
  observer.observe(body)
  onCleanup(() => {
    el.removeEventListener('scroll', schedule)
    observer.disconnect()
    cancelAnimationFrame(frame)
  })
}, { flush: 'post' })
</script>

<template>
  <section :ref="setScroller" class="messages-section scroll-area min-w-0 flex-1 overflow-y-auto overflow-x-hidden px-4 py-6" :class="{ 'has-history-nav': history.length > 1 }">
    <div ref="content" class="flex min-w-0 flex-col gap-5 max-w-[720px] mx-auto">
      <div
        v-if="loadingOlderHistory"
        class="flex justify-center py-2"
      >
        <svg class="loading-spinner-accent animate-spin w-5 h-5" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
        </svg>
      </div>
      <div v-for="item in messages" :key="item.id" :data-message-id="item.id" :data-history-message="item.role === 'user' ? item.id : undefined" :class="{ 'search-message-focus': item.id === focusedMessageId }" class="message-anchor min-w-0">
        <ChatMessageItem :item="item" />
      </div>
      <button v-if="hasNewerHistory" type="button" class="search-latest-link" :disabled="loadingNewerMessages" @click="emit('loadNewer')">{{ t('chatSearchNewer') }}</button>
      <button v-if="hasNewerHistory" type="button" class="search-latest-link" :disabled="loadingNewerMessages" @click="emit('scrollToBottom')">{{ t('chatSearchLatest') }} ↓</button>
    </div>
  </section>

  <ChatHistoryNav v-if="history.length > 1" :messages="history" :active-id="activeId" @jump="jumpToMessage" />

  <Transition name="scroll-bottom-fade">
    <div v-if="showScrollToBottom" class="pointer-events-none absolute right-6 bottom-3 z-20">
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
.has-history-nav { padding-inline-start: 46px; }
@media (min-width: 768px) {
  .has-history-nav { padding-inline-end: 46px; }
}
</style>
