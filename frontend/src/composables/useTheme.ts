import { ref } from 'vue'

const STORAGE_KEY = 'slimebot:theme'
const THEME_SWITCHING_CLASS = 'theme-switching'
const THEME_TRANSITION_DURATION_MS = 220

type Theme = 'dark' | 'light'

function getInitialTheme(): Theme {
  const saved = localStorage.getItem(STORAGE_KEY) as Theme | null
  if (saved === 'dark' || saved === 'light') return saved
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function applyTheme(theme: Theme) {
  if (theme === 'dark') {
    document.documentElement.classList.add('dark')
  } else {
    document.documentElement.classList.remove('dark')
  }
}

const isDark = ref<boolean>(false)
const isThemeTransitioning = ref<boolean>(false)
let themeTransitionTimer: number | null = null

function prefersReducedMotion() {
  return window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
}

function clearThemeTransitionTimer() {
  if (typeof themeTransitionTimer === 'number') {
    window.clearTimeout(themeTransitionTimer)
    themeTransitionTimer = null
  }
}

function setThemeSwitchingClass(active: boolean) {
  if (active) {
    document.documentElement.classList.add(THEME_SWITCHING_CLASS)
    isThemeTransitioning.value = true
    return
  }
  document.documentElement.classList.remove(THEME_SWITCHING_CLASS)
  isThemeTransitioning.value = false
}

function startThemeTransition() {
  clearThemeTransitionTimer()

  if (prefersReducedMotion()) {
    setThemeSwitchingClass(false)
    return
  }

  setThemeSwitchingClass(true)
  themeTransitionTimer = window.setTimeout(() => {
    setThemeSwitchingClass(false)
    themeTransitionTimer = null
  }, THEME_TRANSITION_DURATION_MS)
}

function setTheme(theme: Theme, options?: { persist?: boolean; animate?: boolean }) {
  if (options?.animate) {
    startThemeTransition()
  }
  isDark.value = theme === 'dark'
  applyTheme(theme)
  if (options?.persist) {
    localStorage.setItem(STORAGE_KEY, theme)
  }
}

export function useTheme() {
  function init() {
    const theme = getInitialTheme()
    setTheme(theme)
  }

  function toggleTheme() {
    const nextTheme: Theme = isDark.value ? 'light' : 'dark'
    setTheme(nextTheme, { persist: true, animate: true })
  }

  return {
    isDark,
    isThemeTransitioning,
    toggleTheme,
    init,
    THEME_TRANSITION_DURATION_MS,
  }
}
