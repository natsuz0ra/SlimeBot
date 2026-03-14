<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

type AuroraBand = {
  topHue: number
  bottomHue: number
  alpha: number
  baseY: number
  amplitude: number
  thickness: number
  speed: number
  phase: number
}

const canvasRef = ref<HTMLCanvasElement | null>(null)

let rafId: number | null = null
let lastTime = 0
let elapsed = 0
let isRunning = false
let isDark = false
let prefersReducedMotion = false

let resizeObserver: ResizeObserver | null = null
let motionMediaQuery: MediaQueryList | null = null
let classObserver: MutationObserver | null = null
let motionQueryHandler: ((event: MediaQueryListEvent) => void) | null = null

const MAX_DPR = 2.5

const lightBands: AuroraBand[] = [
  { topHue: 248, bottomHue: 270, alpha: 0.24, baseY: 0.18, amplitude: 0.055, thickness: 0.2, speed: 0.17, phase: 0.2 },
  { topHue: 200, bottomHue: 235, alpha: 0.18, baseY: 0.36, amplitude: 0.05, thickness: 0.18, speed: 0.13, phase: 1.1 },
  { topHue: 276, bottomHue: 308, alpha: 0.14, baseY: 0.56, amplitude: 0.045, thickness: 0.16, speed: 0.1, phase: 2.4 },
]

const darkBands: AuroraBand[] = [
  { topHue: 212, bottomHue: 252, alpha: 0.3, baseY: 0.2, amplitude: 0.06, thickness: 0.22, speed: 0.2, phase: 0.3 },
  { topHue: 262, bottomHue: 292, alpha: 0.25, baseY: 0.38, amplitude: 0.052, thickness: 0.2, speed: 0.14, phase: 1.35 },
  { topHue: 188, bottomHue: 228, alpha: 0.2, baseY: 0.58, amplitude: 0.046, thickness: 0.17, speed: 0.11, phase: 2.6 },
]

function updateThemeMode() {
  isDark = document.documentElement.classList.contains('dark') || document.body.classList.contains('dark')
}

function syncCanvasSize() {
  const canvas = canvasRef.value
  if (!canvas) return
  const parent = canvas.parentElement
  if (!parent) return

  const rect = parent.getBoundingClientRect()
  const width = Math.max(1, rect.width)
  const height = Math.max(1, rect.height)
  const dpr = Math.min(window.devicePixelRatio || 1, MAX_DPR)

  canvas.width = Math.round(width * dpr)
  canvas.height = Math.round(height * dpr)
  canvas.style.width = `${Math.round(width)}px`
  canvas.style.height = `${Math.round(height)}px`
}

function drawBand(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  time: number,
  band: AuroraBand,
) {
  const centerY = height * band.baseY
  const amplitude = height * band.amplitude
  const thickness = height * band.thickness
  const segments = Math.max(16, Math.round(width / 120))
  const step = width / segments

  const gradient = ctx.createLinearGradient(0, centerY - thickness * 0.5, width, centerY + thickness * 0.8)
  gradient.addColorStop(0, `hsla(${band.topHue}, 100%, 72%, 0)`)
  gradient.addColorStop(0.2, `hsla(${band.topHue}, 96%, 70%, ${band.alpha * 0.9})`)
  gradient.addColorStop(0.55, `hsla(${band.bottomHue}, 98%, 67%, ${band.alpha})`)
  gradient.addColorStop(0.9, `hsla(${band.topHue}, 92%, 64%, ${band.alpha * 0.56})`)
  gradient.addColorStop(1, `hsla(${band.bottomHue}, 95%, 62%, 0)`)

  ctx.beginPath()
  ctx.moveTo(0, centerY)
  for (let i = 0; i <= segments; i += 1) {
    const x = i * step
    const wave = Math.sin(time * band.speed + band.phase + i * 0.65) * amplitude
    const drift = Math.sin(time * (band.speed * 0.52) + i * 0.48) * amplitude * 0.5
    ctx.lineTo(x, centerY + wave + drift)
  }

  for (let i = segments; i >= 0; i -= 1) {
    const x = i * step
    const wave = Math.sin(time * band.speed + band.phase + i * 0.65) * amplitude
    const drift = Math.sin(time * (band.speed * 0.52) + i * 0.48) * amplitude * 0.5
    const lowerWave = Math.cos(time * (band.speed * 0.74) + band.phase * 1.3 + i * 0.42) * amplitude * 0.28
    ctx.lineTo(x, centerY + wave + drift + thickness + lowerWave)
  }
  ctx.closePath()

  ctx.fillStyle = gradient
  ctx.fill()
}

function drawFrame(timeStamp: number) {
  const canvas = canvasRef.value
  if (!canvas) return
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  const dpr = Math.min(window.devicePixelRatio || 1, MAX_DPR)
  const width = canvas.width / dpr
  const height = canvas.height / dpr
  if (width <= 0 || height <= 0) return

  const delta = Math.min(64, Math.max(0, timeStamp - lastTime))
  lastTime = timeStamp
  elapsed += delta
  const time = elapsed * 0.001

  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.clearRect(0, 0, width, height)

  const bands = isDark ? darkBands : lightBands

  ctx.save()
  ctx.globalCompositeOperation = 'screen'
  ctx.filter = `blur(${isDark ? 24 : 18}px) saturate(${isDark ? 1.12 : 1.04})`
  for (const band of bands) {
    drawBand(ctx, width, height, time, band)
  }
  ctx.restore()

  const overlay = ctx.createLinearGradient(0, 0, 0, height)
  if (isDark) {
    overlay.addColorStop(0, 'rgba(8, 10, 20, 0.26)')
    overlay.addColorStop(0.6, 'rgba(8, 10, 20, 0.14)')
    overlay.addColorStop(1, 'rgba(8, 10, 20, 0.3)')
  } else {
    overlay.addColorStop(0, 'rgba(255, 255, 255, 0.14)')
    overlay.addColorStop(0.6, 'rgba(255, 255, 255, 0.04)')
    overlay.addColorStop(1, 'rgba(255, 255, 255, 0.18)')
  }
  ctx.fillStyle = overlay
  ctx.fillRect(0, 0, width, height)
}

function renderStaticFrame() {
  elapsed = 0
  drawFrame(16)
}

function stopAnimation() {
  if (rafId !== null) {
    cancelAnimationFrame(rafId)
    rafId = null
  }
  isRunning = false
}

function startAnimation() {
  if (isRunning || prefersReducedMotion || document.hidden) return
  isRunning = true
  lastTime = performance.now()
  const loop = (timeStamp: number) => {
    drawFrame(timeStamp)
    if (!isRunning) return
    rafId = requestAnimationFrame(loop)
  }
  rafId = requestAnimationFrame(loop)
}

function handleVisibilityChange() {
  if (document.hidden) {
    stopAnimation()
    return
  }
  if (prefersReducedMotion) {
    renderStaticFrame()
    return
  }
  startAnimation()
}

function applyMotionPreference(matches: boolean) {
  prefersReducedMotion = matches
  if (matches) {
    stopAnimation()
    renderStaticFrame()
  } else {
    startAnimation()
  }
}

onMounted(() => {
  updateThemeMode()
  syncCanvasSize()

  motionMediaQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
  applyMotionPreference(motionMediaQuery.matches)

  motionQueryHandler = (event: MediaQueryListEvent) => applyMotionPreference(event.matches)
  motionMediaQuery.addEventListener('change', motionQueryHandler)
  document.addEventListener('visibilitychange', handleVisibilityChange)

  resizeObserver = new ResizeObserver(() => {
    syncCanvasSize()
    if (prefersReducedMotion) renderStaticFrame()
  })
  if (canvasRef.value?.parentElement) {
    resizeObserver.observe(canvasRef.value.parentElement)
  }

  classObserver = new MutationObserver(() => {
    updateThemeMode()
    if (prefersReducedMotion) {
      renderStaticFrame()
    }
  })
  classObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
  classObserver.observe(document.body, { attributes: true, attributeFilter: ['class'] })

  if (!prefersReducedMotion) {
    startAnimation()
  } else {
    renderStaticFrame()
  }
})

onBeforeUnmount(() => {
  stopAnimation()

  if (motionMediaQuery) {
    if (motionQueryHandler) {
      motionMediaQuery.removeEventListener('change', motionQueryHandler)
    }
    motionQueryHandler = null
  }
  document.removeEventListener('visibilitychange', handleVisibilityChange)

  if (resizeObserver) {
    resizeObserver.disconnect()
    resizeObserver = null
  }
  if (classObserver) {
    classObserver.disconnect()
    classObserver = null
  }
})
</script>

<template>
  <canvas ref="canvasRef" class="login-aurora-canvas" aria-hidden="true" />
</template>

<style scoped>
.login-aurora-canvas {
  position: absolute;
  inset: 0;
  display: block;
  width: 100%;
  height: 100%;
  pointer-events: none;
}
</style>
