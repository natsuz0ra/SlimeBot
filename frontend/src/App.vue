<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import AppToast from '@/components/ui/AppToast.vue'
import { useTheme } from '@/composables/useTheme'

const { init } = useTheme()
const route = useRoute()
const LOGIN_HOME_TRANSITION_TOKEN = 'slimebot:transition:login-home'
const shouldPlayLoginToHome = ref(false)
const skipFadeForChatSwitch = ref(false)

function hasLoginToHomeToken() {
  try {
    return sessionStorage.getItem(LOGIN_HOME_TRANSITION_TOKEN) === '1'
  } catch {
    return false
  }
}

function clearLoginToHomeToken() {
  try {
    sessionStorage.removeItem(LOGIN_HOME_TRANSITION_TOKEN)
  } catch {
    // Ignore storage access errors in private mode.
  }
}

function normalizePath(pathLike: string | undefined) {
  if (!pathLike) return ''
  return pathLike.split('?')[0]?.split('#')[0] ?? ''
}

function isChatPath(pathLike: string | undefined) {
  const normalized = normalizePath(pathLike)
  return normalized.startsWith('/chat/')
}

function resolveRouteComponentKey(pathLike: string | undefined) {
  if (isChatPath(pathLike)) return 'chat-home'
  return pathLike || ''
}

const routeTransitionName = computed(() => {
  if (skipFadeForChatSwitch.value) return 'route-none'
  return shouldPlayLoginToHome.value ? 'route-login-to-home' : 'route-fade'
})
const routeTransitionMode = computed<'out-in' | undefined>(() => (skipFadeForChatSwitch.value ? undefined : 'out-in'))

watch(
  () => route.fullPath,
  (fullPath, previousFullPath) => {
    shouldPlayLoginToHome.value = route.path === '/' && hasLoginToHomeToken()
    skipFadeForChatSwitch.value = isChatPath(previousFullPath) && isChatPath(fullPath)
  },
  { immediate: true },
)

function onRouteEnterDone() {
  if (!shouldPlayLoginToHome.value) return
  shouldPlayLoginToHome.value = false
  clearLoginToHomeToken()
}

onMounted(() => init())
</script>

<template>
  <router-view v-slot="{ Component, route: currentRoute }">
    <Transition :name="routeTransitionName" :mode="routeTransitionMode" @after-enter="onRouteEnterDone">
      <component :is="Component" :key="resolveRouteComponentKey(currentRoute.fullPath)" />
    </Transition>
  </router-view>
  <AppToast />
</template>

<style>
.route-fade-enter-active,
.route-fade-leave-active {
  transition: opacity 120ms ease;
}

.route-fade-enter-from,
.route-fade-leave-to {
  opacity: 0;
}

.route-none-enter-active,
.route-none-leave-active {
  transition: none;
}

.route-none-enter-from,
.route-none-leave-to {
  opacity: 1;
}

.route-login-to-home-leave-active {
  transition: opacity 150ms ease;
}

.route-login-to-home-leave-from {
  opacity: 1;
}

.route-login-to-home-leave-to {
  opacity: 0;
}

.route-login-to-home-enter-active {
  transition: opacity 220ms cubic-bezier(0.22, 1, 0.36, 1);
}

.route-login-to-home-enter-from {
  opacity: 0;
}

.route-login-to-home-enter-to {
  opacity: 1;
}

@media (prefers-reduced-motion: reduce) {
  .route-fade-enter-active,
  .route-fade-leave-active,
  .route-login-to-home-enter-active,
  .route-login-to-home-leave-active {
    transition: opacity 80ms linear !important;
  }

  .route-login-to-home-enter-from,
  .route-login-to-home-leave-to {
    transform: none !important;
  }
}
</style>
