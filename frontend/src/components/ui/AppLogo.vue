<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    size?: number
    white?: boolean
    animated?: boolean
  }>(),
  {
    size: 24,
    white: false,
    animated: false,
  },
)

const idPrefix = `slime-${Math.random().toString(36).slice(2, 10)}`
const bodyGradientId = `${idPrefix}-body-gradient`
const highlightGradientId = `${idPrefix}-highlight-gradient`
const shadowFilterId = `${idPrefix}-shadow`

const logoStyle = computed(() => (props.white ? 'filter: brightness(0) invert(1)' : ''))
</script>

<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 100 100"
    fill="none"
    xmlns="http://www.w3.org/2000/svg"
    class="slimebot-logo block object-contain"
    :class="{ 'is-animated': animated }"
    :style="logoStyle"
    role="img"
    aria-label="SlimeBot"
  >
    <defs>
      <linearGradient
        :id="bodyGradientId"
        x1="24"
        y1="20"
        x2="76"
        y2="82"
        gradientUnits="userSpaceOnUse"
        gradientTransform="translate(50 50) scale(1.14) translate(-50 -50)"
      >
        <stop offset="0" stop-color="#6366F1" />
        <stop offset="0.55" stop-color="#A78BFA" />
        <stop offset="1" stop-color="#818CF8" />
      </linearGradient>
      <linearGradient
        :id="highlightGradientId"
        x1="33"
        y1="26"
        x2="60"
        y2="52"
        gradientUnits="userSpaceOnUse"
        gradientTransform="translate(50 50) scale(1.14) translate(-50 -50)"
      >
        <stop offset="0" stop-color="#FFFFFF" stop-opacity="0.88" />
        <stop offset="1" stop-color="#FFFFFF" stop-opacity="0" />
      </linearGradient>
      <filter :id="shadowFilterId" x="6" y="60" width="88" height="30" filterUnits="userSpaceOnUse">
        <feGaussianBlur in="SourceGraphic" stdDeviation="3.2" />
      </filter>
    </defs>
    <g transform="translate(50 50) scale(1.14) translate(-50 -50)">
      <ellipse cx="50" cy="76" rx="22" ry="6" fill="#4338CA" fill-opacity="0.18" :filter="`url(#${shadowFilterId})`" />
      <path
        d="M50 16C61.4 16 71.12 20.12 76.86 27.14C81.14 32.38 83.2 38.95 82.87 46.85C82.52 55.1 80.39 61.65 76.48 67.2C71.09 74.89 62.41 80.02 52.52 81.79C51.04 82.05 49.56 82.18 48.09 82.18C42.46 82.18 37.08 80.43 32.26 78.06C24.55 74.28 18.86 67.81 17.34 58.45C16.09 50.69 17.5 43.55 20.42 37.55C22.77 32.72 26.21 28.41 30.49 25.02C35.96 20.69 42.73 16.75 50 16Z"
        :fill="`url(#${bodyGradientId})`"
      />
      <path
        d="M33.2 28.3C37.11 23.95 43.72 21.35 50.44 21.35C56.43 21.35 61.53 22.54 65.51 25.46C60.77 24.57 57.15 24.81 53.73 25.85C46.91 27.92 41.91 33.11 39.22 39.25C38.75 40.32 37.48 40.8 36.45 40.26C34.45 39.2 33 37.45 32.54 35.22C32.08 33 32.32 30.88 33.2 28.3Z"
        :fill="`url(#${highlightGradientId})`"
      />
      <rect class="slimebot-logo__eye" x="37" y="42" width="7" height="18" rx="3.5" fill="#FFFFFF" />
      <rect class="slimebot-logo__eye" x="56" y="42" width="7" height="18" rx="3.5" fill="#FFFFFF" />
      <path
        d="M28.78 66.37C32.3 71.57 39.43 75.98 50 75.98C60.55 75.98 67.7 71.57 71.22 66.37C67.57 74.55 59.82 79.7 50 79.7C40.18 79.7 32.43 74.55 28.78 66.37Z"
        fill="#312E81"
        fill-opacity="0.16"
      />
    </g>
  </svg>
</template>

<style scoped>
.slimebot-logo__eye {
  transform-box: fill-box;
  transform-origin: center;
}
</style>
