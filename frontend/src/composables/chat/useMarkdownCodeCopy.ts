import { useI18n } from 'vue-i18n'
import { useToast } from '@/composables/useToast'
import { inject } from 'vue'
import { CHAT_CONTEXT_KEY } from './useChatContext'

export function useMarkdownCodeCopy() {
  const { t } = useI18n()
  const toast = useToast()
  const sendToTerminal = inject(CHAT_CONTEXT_KEY, undefined)?.sendToTerminal

  async function handleMarkdownClick(event: MouseEvent) {
    const target = event.target instanceof Element ? event.target : null
    const button = target?.closest<HTMLButtonElement>('.markdown-code-copy-btn, .markdown-code-terminal-btn')
    if (!button) return
    const codeBlock = button.closest('.markdown-code-block')
    const code = (codeBlock?.querySelector('pre code')?.textContent ?? '').replace(/\n+$/, '')
    if (!code) return
    if (button.dataset.codeLang) {
      sendToTerminal?.(code, button.dataset.codeLang)
      return
    }
    try {
      await navigator.clipboard.writeText(code)
      toast.success(t('messageCopySuccess'))
    } catch {
      toast.error(t('messageCopyFailed'))
    }
  }

  return { handleMarkdownClick, sendToTerminal }
}
