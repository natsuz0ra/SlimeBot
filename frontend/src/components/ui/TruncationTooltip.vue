<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { CSSProperties } from 'vue'

const HOVER_DELAY_MS = 450
const VIEWPORT_MARGIN_PX = 8
const TOOLTIP_GAP_PX = 8
const TOOLTIP_MAX_WIDTH_PX = 360
const TOOLTIP_Z_INDEX = 9999
const ARROW_EDGE_PADDING_PX = 14

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
const tooltipRef = ref<HTMLElement | null>(null)
const overflow = ref(false)
const visible = ref(false)
const placement = ref<'top' | 'bottom'>('top')
const tooltipStyle = ref<CSSProperties>({
  left: '0px',
  top: '0px',
  maxWidth: `${TOOLTIP_MAX_WIDTH_PX}px`,
})
const arrowStyle = ref<CSSProperties>({
  left: '50%',
})

let hoverTimer: ReturnType<typeof setTimeout> | null = null
let boundAnchor: HTMLElement | null = null
let hoverRaf = 0
let positionRaf = 0
let scrollParents: HTMLElement[] = []

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

function cancelPositionRaf() {
  if (positionRaf) {
    cancelAnimationFrame(positionRaf)
    positionRaf = 0
  }
}

function getHoverAnchor(): HTMLElement | null {
  const el = rootRef.value
  if (!el) return null
  return props.inheritGroup ? (el.parentElement as HTMLElement | null) : el
}

function getScrollParents(el: HTMLElement | null) {
  const parents: HTMLElement[] = []
  let current = el?.parentElement ?? null

  while (current && current !== document.body) {
    const style = window.getComputedStyle(current)
    const overflowValue = `${style.overflow}${style.overflowX}${style.overflowY}`
    if (/(auto|scroll|overlay)/.test(overflowValue)) {
      parents.push(current)
    }
    current = current.parentElement
  }

  return parents
}

function schedulePositionUpdate() {
  if (!visible.value) return
  cancelPositionRaf()
  positionRaf = window.requestAnimationFrame(() => {
    positionRaf = 0
    updatePosition()
  })
}

function updatePosition() {
  const anchor = getHoverAnchor()
  const tooltip = tooltipRef.value
  if (!anchor || !tooltip || !visible.value) return

  const anchorRect = anchor.getBoundingClientRect()
  const tooltipRect = tooltip.getBoundingClientRect()
  const maxWidth = Math.min(TOOLTIP_MAX_WIDTH_PX, window.innerWidth - VIEWPORT_MARGIN_PX * 2)
  const centeredLeft = anchorRect.left + anchorRect.width / 2 - tooltipRect.width / 2
  const left = Math.max(
    VIEWPORT_MARGIN_PX,
    Math.min(centeredLeft, window.innerWidth - VIEWPORT_MARGIN_PX - tooltipRect.width),
  )
  const topCandidate = anchorRect.top - tooltipRect.height - TOOLTIP_GAP_PX
  const bottomCandidate = anchorRect.bottom + TOOLTIP_GAP_PX
  const canPlaceTop = topCandidate >= VIEWPORT_MARGIN_PX
  const canPlaceBottom = bottomCandidate + tooltipRect.height <= window.innerHeight - VIEWPORT_MARGIN_PX

  placement.value = canPlaceTop || !canPlaceBottom ? 'top' : 'bottom'

  const top =
    placement.value === 'top'
      ? Math.max(VIEWPORT_MARGIN_PX, topCandidate)
      : Math.min(bottomCandidate, window.innerHeight - VIEWPORT_MARGIN_PX - tooltipRect.height)

  const anchorCenterX = anchorRect.left + anchorRect.width / 2
  const arrowLeft = Math.max(
    ARROW_EDGE_PADDING_PX,
    Math.min(anchorCenterX - left, tooltipRect.width - ARROW_EDGE_PADDING_PX),
  )

  tooltipStyle.value = {
    left: `${Math.round(left)}px`,
    top: `${Math.round(top)}px`,
    maxWidth: `${Math.round(maxWidth)}px`,
  }
  arrowStyle.value = {
    left: `${Math.round(arrowLeft)}px`,
  }
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

function bindResizeObserver() {
  ro?.disconnect()
  ro = new ResizeObserver(() => {
    measure()
    schedulePositionUpdate()
  })
  if (lineRef.value) ro.observe(lineRef.value)
  const anchor = getHoverAnchor()
  if (anchor) {
    ro.observe(anchor)
  }
  if (tooltipRef.value) ro.observe(tooltipRef.value)
}

function bindPositionListeners() {
  unbindPositionListeners()
  window.addEventListener('resize', schedulePositionUpdate)
  window.addEventListener('scroll', schedulePositionUpdate, true)
  scrollParents = getScrollParents(getHoverAnchor())
  for (const parent of scrollParents) {
    parent.addEventListener('scroll', schedulePositionUpdate, { passive: true })
  }
}

function unbindPositionListeners() {
  window.removeEventListener('resize', schedulePositionUpdate)
  window.removeEventListener('scroll', schedulePositionUpdate, true)
  for (const parent of scrollParents) {
    parent.removeEventListener('scroll', schedulePositionUpdate)
  }
  scrollParents = []
}

onMounted(() => {
  void nextTick(() => {
    measure()
    bindHover()
    bindResizeObserver()
  })
})

watch(
  () => props.text,
  () => {
    void nextTick(() => {
      measure()
      schedulePositionUpdate()
    })
  },
)

watch(
  () => props.inheritGroup,
  () => {
    void nextTick(() => {
      unbindHover()
      bindHover()
      bindResizeObserver()
      schedulePositionUpdate()
    })
  },
)

watch(visible, (value) => {
  cancelAnimationFrame(hoverRaf)
  if (!value) {
    unbindPositionListeners()
    return
  }

  void nextTick(() => {
    bindResizeObserver()
    bindPositionListeners()
    updatePosition()
    hoverRaf = window.requestAnimationFrame(() => {
      hoverRaf = 0
      updatePosition()
    })
  })
})

onBeforeUnmount(() => {
  unbindHover()
  unbindPositionListeners()
  clearHoverTimer()
  cancelAnimationFrame(hoverRaf)
  cancelPositionRaf()
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
  </span>
  <Teleport to="body">
    <div
      v-if="overflow && visible"
      ref="tooltipRef"
      class="pointer-events-none fixed w-max rounded-lg bg-black/78 px-3 py-2 text-left text-sm leading-5 text-white shadow-lg"
      :style="{ ...tooltipStyle, zIndex: TOOLTIP_Z_INDEX }"
    >
      <span class="break-words">{{ text }}</span>
      <div
        class="absolute h-2 w-2 -translate-x-1/2 rotate-45 bg-black/78"
        :class="placement === 'top' ? '-bottom-1' : '-top-1'"
        :style="arrowStyle"
      />
    </div>
  </Teleport>
</template>
