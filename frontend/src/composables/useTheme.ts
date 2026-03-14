import { ref, watch } from 'vue'

const STORAGE_KEY = 'slimebot:theme'

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

export function useTheme() {
  function init() {
    const theme = getInitialTheme()
    isDark.value = theme === 'dark'
    applyTheme(theme)
  }

  function toggleTheme() {
    isDark.value = !isDark.value
    const theme: Theme = isDark.value ? 'dark' : 'light'
    applyTheme(theme)
    localStorage.setItem(STORAGE_KEY, theme)
  }

  watch(isDark, (val) => {
    applyTheme(val ? 'dark' : 'light')
  })

  return { isDark, toggleTheme, init }
}
