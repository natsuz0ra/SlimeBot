<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MessageItem } from '@/api/chat'

const props = defineProps<{
  messages: MessageItem[]
  activeId: string | null
}>()
const emit = defineEmits<{ jump: [id: string] }>()
const { t } = useI18n()
const markers = ref<HTMLElement | null>(null)
let resolution: MediaQueryList | undefined
let resizeObserver: ResizeObserver | undefined

function updateDevicePixel() {
  resolution?.removeEventListener('change', updateDevicePixel)
  const ratio = window.devicePixelRatio
  markers.value?.style.setProperty('--history-device-pixel', `${1 / ratio}px`)
  markers.value?.style.setProperty('--history-stroke', `${Math.max(1, Math.round(1.5 * ratio)) / ratio}px`)
  alignMarkerStrokes()
  resolution = window.matchMedia(`(resolution: ${ratio}dppx)`)
  resolution.addEventListener('change', updateDevicePixel)
}

function alignMarkerStrokes() {
  const list = markers.value
  const firstMarker = list?.querySelector<HTMLElement>('.history-marker')
  if (!list || !firstMarker) return
  const { top, height } = firstMarker.getBoundingClientRect()
  list.style.setProperty('--history-first-center', `${top + height / 2}px`)
}

onMounted(() => {
  updateDevicePixel()
  const list = markers.value
  if (!list || typeof ResizeObserver === 'undefined') return
  resizeObserver = new ResizeObserver(alignMarkerStrokes)
  resizeObserver.observe(list)
  const container = list.closest('.chat-content-scroll')
  if (container) resizeObserver.observe(container)
})
onUnmounted(() => {
  resolution?.removeEventListener('change', updateDevicePixel)
  resizeObserver?.disconnect()
})

type PreviewMarker = { id: string; top: number }
const hoveredIndex = ref<number>()
const hoveredMarker = ref<PreviewMarker | null>(null)
const focusedMarker = ref<PreviewMarker | null>(null)
const previewMarker = computed(() => hoveredMarker.value ?? focusedMarker.value)
const preview = computed(() => props.messages.find(message => message.id === previewMarker.value?.id))

function summary(message: MessageItem) {
  const text = message.content.trim().replace(/\s+/g, ' ')
  return text.slice(0, 180) || message.attachments?.map(file => file.name).join(', ').slice(0, 180) || t('chatHistoryMessage')
}

function markerPosition(id: string, event: Event): PreviewMarker {
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const listTop = markers.value?.getBoundingClientRect().top ?? rect.top
  return { id, top: rect.top - listTop + rect.height / 2 }
}

function hoverMarker(id: string, index: number, event: PointerEvent) {
  if (event.pointerType === 'touch') return
  hoveredIndex.value = index
  hoveredMarker.value = markerPosition(id, event)
}

function clearHover() {
  hoveredIndex.value = undefined
  hoveredMarker.value = null
}

function dismissPreview() {
  hoveredMarker.value = null
  focusedMarker.value = null
}

function onMarkerScroll() {
  alignMarkerStrokes()
  hoveredIndex.value = undefined
  dismissPreview()
}

function jump(id: string) {
  dismissPreview()
  emit('jump', id)
}

watch(() => props.activeId, () => {
  const list = markers.value
  const marker = list?.querySelector<HTMLElement>('[aria-current="location"]')
  if (!list || !marker || hoveredIndex.value !== undefined || previewMarker.value) return
  const { top, bottom } = marker.getBoundingClientRect()
  const visible = list.getBoundingClientRect()
  if (top < visible.top) list.scrollTop += top - visible.top
  if (bottom > visible.bottom) list.scrollTop += bottom - visible.bottom
}, { flush: 'post' })
</script>

<template>
  <nav class="chat-history-nav" :aria-label="t('chatHistoryNav')" @keydown.esc="dismissPreview">
    <ol
      ref="markers"
      class="history-markers"
      :style="{ '--history-highlight': hoveredIndex }"
      @pointerleave="clearHover"
      @scroll="onMarkerScroll"
    >
      <li v-for="(message, index) in messages" :key="message.id">
        <button
          type="button"
          class="history-marker"
          :style="{ '--history-index': index }"
          :aria-current="message.id === activeId ? 'location' : undefined"
          :aria-label="t('chatHistoryJump', { number: index + 1, text: summary(message) })"
          @pointerenter="hoverMarker(message.id, index, $event)"
          @focus="focusedMarker = markerPosition(message.id, $event)"
          @blur="focusedMarker = null"
          @click="jump(message.id)"
        ><span aria-hidden="true" /></button>
      </li>
    </ol>
    <Transition name="history-preview">
      <div v-if="preview && previewMarker" class="history-preview" :style="{ top: `${previewMarker.top}px` }" aria-hidden="true">
        <p>{{ summary(preview) }}</p>
      </div>
    </Transition>
  </nav>
</template>

<style scoped>
.chat-history-nav {
  position: absolute;
  z-index: 5;
  inset-inline-start: 6px;
  inset-block: 0;
  height: max-content;
  margin-block: auto;
  width: 36px;
  max-height: calc(100% - 24px);
}

.history-markers {
  max-height: min(38vh, 280px);
  overflow-y: auto;
  overflow-x: hidden;
  overscroll-behavior: contain;
  scrollbar-width: none;
  padding-block: 4px;
}

.history-marker {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 24px;
  border-radius: 6px;
  cursor: pointer;
}

.history-marker span {
  width: 12px;
  height: 3px;
  border-radius: 999px;
  background: var(--history-muted, var(--text-muted));
}

.history-marker:focus-visible { outline: 2px solid var(--sb-brand); outline-offset: -2px; }
.history-marker[aria-current] span { background: var(--sb-brand); }

@media (hover: hover) and (pointer: fine) {
  .history-markers {
    --history-step: 11px;
  }

  @supports (color: oklch(from purple l 0 h)) {
    .history-markers {
      /* Soften the neutral bars against the current theme. */
      --history-muted: color-mix(in oklab, oklch(from var(--sb-brand) l 0 h) 75%, var(--bg-main));
    }
  }

  .history-marker {
    --history-offset: calc(var(--history-index) - var(--history-highlight, -999));
    --history-distance: max(var(--history-offset), calc(-1 * var(--history-offset)));
    --history-emphasis: max(0, calc(1 - var(--history-distance) / 4));
    justify-content: flex-start;
    height: var(--history-step);
    padding-inline-start: 5px;
  }

  .history-marker span {
    /* Snap the painted bar to the screen, leaving its button in the original row. */
    --history-stroke-top: calc(var(--history-first-center, 0px) + var(--history-index) * var(--history-step) - var(--history-stroke, 2px) / 2);
    translate: 0 calc(round(nearest, var(--history-stroke-top), var(--history-device-pixel, 1px)) - var(--history-stroke-top));
    width: calc(8px + 22px * var(--history-emphasis));
    height: var(--history-stroke, 2px);
    border-radius: 0;
    transition: width 150ms cubic-bezier(0.22, 1, 0.36, 1);
  }

  .history-markers:not(:hover) .history-marker[aria-current] span { width: 16px; }
  .history-markers:hover .history-marker[aria-current] span { background: var(--history-muted, var(--text-muted)); }
  .history-markers:hover .history-marker:hover span { background: var(--sb-brand); }
}

.history-preview {
  position: absolute;
  inset-inline-start: calc(100% + 6px);
  width: min(260px, calc(100vw - 76px));
  transform: translateY(-50%);
  border: 1px solid var(--menu-border);
  border-radius: 10px;
  background: var(--bg-main);
  box-shadow: var(--menu-shadow);
  padding: 10px 12px;
  color: var(--text-primary);
  font-size: 13px;
  line-height: 1.45;
  pointer-events: none;
}

.history-preview p {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
  overflow: hidden;
  overflow-wrap: anywhere;
}

.history-preview-enter-active,
.history-preview-leave-active {
  transition: opacity 130ms ease-out, transform 160ms cubic-bezier(0.22, 1, 0.36, 1);
}

.history-preview-enter-from,
.history-preview-leave-to {
  opacity: 0;
  transform: translate(-4px, -50%);
}

@media (prefers-reduced-motion: reduce) {
  .history-marker span,
  .history-preview-enter-active,
  .history-preview-leave-active { transition: none; }
}
</style>
