<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch, nextTick } from 'vue'

const HOVER_DELAY_MS = 450

const props = withDefaults(
  defineProps<{
    text: string
    contentClass?: string
    wrapperClass?: string
    inheritGroup?: boolean
  }>(),
  {
    contentClass: '',
    wrapperClass: '',
    inheritGroup: false,
  },
)

const rootRef = ref<HTMLElement | null>(null)
const lineRef = ref<HTMLElement | null>(null)
const overflow = ref(false)
const visible = ref(false)

let hoverTimer: ReturnType<typeof setTimeout> | null = null
let boundAnchor: HTMLElement | null = null

function measure() {
  const el = lineRef.value
  if (!el) return
  overflow.value = el.scrollWidth > el.clientWidth + 1
  if (!overflow.value) visible.value = false
}

function clearHoverTimer() {
  if (hoverTimer != null) {
    clearTimeout(hoverTimer)
    hoverTimer = null
  }
}

function getHoverAnchor(): HTMLElement | null {
  const el = rootRef.value
  if (!el) return null
  return props.inheritGroup ? (el.parentElement as HTMLElement | null) : el
}

function onMouseEnter() {
  if (!overflow.value) return
  clearHoverTimer()
  hoverTimer = window.setTimeout(() => {
    visible.value = true
    hoverTimer = null
  }, HOVER_DELAY_MS)
}

function onMouseLeave() {
  clearHoverTimer()
  visible.value = false
}

function bindHover() {
  unbindHover()
  const anchor = getHoverAnchor()
  if (!anchor) return
  anchor.addEventListener('mouseenter', onMouseEnter)
  anchor.addEventListener('mouseleave', onMouseLeave)
  boundAnchor = anchor
}

function unbindHover() {
  if (boundAnchor) {
    boundAnchor.removeEventListener('mouseenter', onMouseEnter)
    boundAnchor.removeEventListener('mouseleave', onMouseLeave)
    boundAnchor = null
  }
}

let ro: ResizeObserver | null = null

onMounted(() => {
  void nextTick(() => {
    measure()
    bindHover()
    ro = new ResizeObserver(() => measure())
    if (lineRef.value) ro.observe(lineRef.value)
  })
})

watch(() => props.text, () => void nextTick(measure))

watch(
  () => props.inheritGroup,
  () => {
    void nextTick(() => {
      unbindHover()
      bindHover()
    })
  },
)

onBeforeUnmount(() => {
  unbindHover()
  clearHoverTimer()
  ro?.disconnect()
})
</script>

<template>
  <span
    ref="rootRef"
    class="relative inline-flex max-w-full min-w-0 align-bottom"
    :class="wrapperClass"
  >
    <span
      ref="lineRef"
      class="block min-w-0 truncate"
      :class="contentClass"
    >{{ text }}</span>
    <div
      v-if="overflow && visible"
      class="pointer-events-none absolute bottom-full left-1/2 z-[300] mb-2 w-max max-w-[min(90vw,360px)] -translate-x-1/2 rounded-lg bg-black/78 px-3 py-2 text-left text-sm leading-5 text-white shadow-lg"
    >
      <span class="break-words">{{ text }}</span>
      <div class="absolute -bottom-1 left-1/2 h-2 w-2 -translate-x-1/2 rotate-45 bg-black/78" />
    </div>
  </span>
</template>
