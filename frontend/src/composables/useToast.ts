import { reactive } from 'vue'

type ToastType = 'success' | 'error' | 'warning' | 'info'

export interface ToastItem {
  id: number
  type: ToastType
  message: string
}

let nextId = 0

export const toastState = reactive<{ items: ToastItem[] }>({ items: [] })

export function useToast() {
  function show(type: ToastType, message: string, duration = 3000) {
    const id = ++nextId
    toastState.items.push({ id, type, message })
    setTimeout(() => {
      const idx = toastState.items.findIndex((t) => t.id === id)
      if (idx !== -1) toastState.items.splice(idx, 1)
    }, duration)
  }

  return {
    success: (msg: string) => show('success', msg),
    error: (msg: string) => show('error', msg),
    warning: (msg: string) => show('warning', msg),
    info: (msg: string) => show('info', msg),
  }
}
