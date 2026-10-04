<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { mdiArrowTopRight, mdiMagnify, mdiMessageTextOutline, mdiTextBoxSearchOutline } from '@mdi/js'
import { sessionAPI, type ChatSearchHit, type ChatSearchScope } from '@/api/chat'
import AppDialog from '@/components/ui/AppDialog.vue'
import MdiIcon from '@/components/ui/MdiIcon.vue'
import { highlightSearchText } from '@/utils/chatSearch'

const props = defineProps<{ visible: boolean; pickResult: (hit: ChatSearchHit) => Promise<void> }>()
const emit = defineEmits<{ close: [] }>()
const { t, locale } = useI18n()
const query = ref('')
const scope = ref<ChatSearchScope>('all')
const hits = ref<ChatSearchHit[]>([])
const hasMore = ref(false)
const loading = ref(false)
const error = ref('')
const opening = ref(false)
const input = ref<HTMLInputElement>()
const results = ref<HTMLElement>()
const scopes = ['all', 'messages', 'titles'] as const
const needle = computed(() => query.value.trim())
const formatter = computed(() => new Intl.DateTimeFormat(locale.value, { month: 'short', day: 'numeric' }))
let timer: ReturnType<typeof setTimeout> | undefined
let request: AbortController | undefined
let generation = 0

function cancelSearch() {
  clearTimeout(timer)
  request?.abort()
  generation++
}

async function search(append = false) {
  const version = generation
  const controller = new AbortController()
  request = controller
  loading.value = true
  error.value = ''
  try {
    const response = await sessionAPI.search(needle.value, scope.value, append ? hits.value.length : 0, controller.signal)
    if (version !== generation) return
    hits.value = append ? [...hits.value, ...response.hits] : response.hits
    hasMore.value = response.hasMore
  } catch {
    if (version === generation && !controller.signal.aborted) error.value = t('chatSearchFailed')
  } finally {
    if (version === generation) loading.value = false
  }
}

watch([query, scope], () => {
  cancelSearch()
  hits.value = []
  hasMore.value = false
  error.value = ''
  loading.value = props.visible && !!needle.value
  if (loading.value) timer = setTimeout(() => void search(), 250)
})
watch(() => props.visible, async (visible) => {
  cancelSearch()
  loading.value = false
  if (visible) {
    if (needle.value) void search()
    await nextTick()
    input.value?.focus()
    input.value?.select()
  }
})
onUnmounted(cancelSearch)

async function pick(hit: ChatSearchHit) {
  if (opening.value) return
  opening.value = true
  error.value = ''
  try {
    await props.pickResult(hit)
    emit('close')
  } catch {
    error.value = t('chatSearchOpenFailed')
  } finally {
    opening.value = false
  }
}

function moveResult(direction: number) {
  const buttons = Array.from(results.value?.querySelectorAll<HTMLButtonElement>('.search-hit') || [])
  const current = buttons.findIndex((button) => button === document.activeElement)
  if (direction < 0 && current <= 0) { input.value?.focus(); return }
  buttons[Math.min(buttons.length - 1, current + direction)]?.focus()
}
</script>

<template>
  <AppDialog :visible="visible" :title="t('chatSearch')" width="680px" hide-footer @cancel="emit('close')">
    <div class="chat-search" @keydown.down.prevent="moveResult(1)" @keydown.up.prevent="moveResult(-1)">
      <div class="search-field">
        <MdiIcon :path="mdiMagnify" :size="21" aria-hidden="true" />
        <input ref="input" v-model="query" autofocus type="search" maxlength="100" autocomplete="off" :aria-label="t('chatSearchPlaceholder')" :placeholder="t('chatSearchPlaceholder')" @keydown.enter.prevent="hits[0] && pick(hits[0])" />
        <span v-if="loading" class="search-spinner" aria-hidden="true" />
      </div>
      <div class="search-toolbar">
        <div class="search-scopes" :aria-label="t('chatSearchScope')">
          <button v-for="item in scopes" :key="item" type="button" :aria-pressed="scope === item" :class="{ active: scope === item }" @click="scope = item">{{ t(`chatSearchScope_${item}`) }}</button>
        </div>
        <span v-if="hits.length" class="search-count">{{ t('chatSearchCount', { count: hits.length }) }}{{ hasMore ? '+' : '' }}</span>
      </div>
      <div ref="results" class="search-results scroll-area" :aria-busy="loading || opening">
        <div v-if="error" class="search-status search-error" role="alert">
          <span>{{ error }}</span>
          <button v-if="!opening" type="button" @click="search()">{{ t('chatSearchRetry') }}</button>
        </div>
        <div v-else-if="!needle || (!loading && !hits.length)" class="search-status">
          <span class="search-empty-icon"><MdiIcon :path="mdiTextBoxSearchOutline" :size="30" aria-hidden="true" /></span>
          <strong>{{ t(needle ? 'chatSearchEmpty' : 'chatSearchHint') }}</strong>
          <span>{{ t(needle ? 'chatSearchEmptyHint' : 'chatSearchHintDetail') }}</span>
        </div>
        <div v-else-if="loading && !hits.length" class="search-status" role="status">{{ t('chatSearchLoading') }}</div>
        <template v-else>
          <button v-for="hit in hits" :key="`${hit.sessionId}:${hit.messageId || 'title'}`" type="button" class="search-hit" :disabled="opening" @click="pick(hit)">
            <span class="search-hit-icon"><MdiIcon :path="mdiMessageTextOutline" :size="18" aria-hidden="true" /></span>
            <span class="search-hit-body">
              <span class="search-hit-heading">
                <span class="search-hit-title"><template v-for="(part, index) in highlightSearchText(hit.sessionName, needle)" :key="index"><mark v-if="part.match">{{ part.text }}</mark><template v-else>{{ part.text }}</template></template></span>
                <time :datetime="hit.createdAt">{{ formatter.format(new Date(hit.createdAt)) }}</time>
              </span>
              <span class="search-hit-meta"><span class="search-hit-kind">{{ t(!hit.messageId ? 'chatSearchTitle' : hit.role === 'user' ? 'chatSearchUser' : 'chatSearchAssistant') }}</span><span>{{ t(hit.messageId ? 'chatSearchGoMessage' : 'chatSearchGoSession') }}</span></span>
              <span v-if="hit.messageId" class="search-snippet"><template v-for="(part, index) in highlightSearchText(hit.snippet, needle)" :key="index"><mark v-if="part.match">{{ part.text }}</mark><template v-else>{{ part.text }}</template></template></span>
            </span>
            <MdiIcon class="search-hit-arrow" :path="mdiArrowTopRight" :size="17" aria-hidden="true" />
          </button>
          <button v-if="hasMore" type="button" class="search-load-more" :disabled="loading || opening" @click="search(true)">{{ t(loading ? 'chatSearchLoading' : 'chatSearchMore') }}</button>
        </template>
      </div>
      <div class="search-footer"><span>{{ t('chatSearchKeyboard') }}</span><span><kbd>Esc</kbd> {{ t('chatSearchClose') }}</span></div>
    </div>
  </AppDialog>
</template>

<style scoped>
.chat-search { color: var(--text-primary); }
.search-field { display: flex; align-items: center; gap: 10px; min-height: 48px; padding: 0 14px; border: 1px solid var(--input-border); border-radius: 12px; background: var(--input-bg); color: var(--text-muted); transition: border-color .15s, box-shadow .15s; }
.search-field:focus-within { border-color: var(--sb-brand); box-shadow: 0 0 0 3px var(--primary-alpha-10); }
.search-field input { flex: 1; min-width: 0; padding: 12px 0; border: 0; outline: 0; background: transparent; color: var(--text-primary); font-size: 14px; }
.search-field input::placeholder { color: var(--text-muted); }
.search-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin: 16px 0 10px; }
.search-scopes { display: flex; gap: 4px; padding: 3px; border-radius: 9px; background: var(--primary-alpha-05); }
.search-scopes button { border: 0; border-radius: 7px; padding: 6px 12px; color: var(--text-secondary); font-size: 12px; cursor: pointer; }
.search-scopes button.active { background: var(--primary-alpha-15); color: var(--sb-brand); font-weight: 600; }
.search-count { color: var(--text-muted); font-size: 12px; white-space: nowrap; }
.search-results { height: min(420px, 50vh); min-height: 200px; overflow: auto; padding: 3px; margin: 0 -3px; }
.search-hit { display: flex; align-items: flex-start; gap: 12px; width: 100%; padding: 14px 12px; margin-bottom: 5px; border: 1px solid transparent; border-radius: 12px; text-align: left; color: inherit; cursor: pointer; transition: background .15s, border-color .15s; }
.search-hit:hover, .search-hit:focus-visible { background: var(--primary-alpha-05); border-color: var(--primary-alpha-20); }
.search-hit:focus-visible, .search-scopes button:focus-visible, .search-load-more:focus-visible { outline: 2px solid var(--sb-brand); outline-offset: 1px; }
.search-hit-icon { flex-shrink: 0; display: flex; align-items: center; justify-content: center; width: 32px; height: 32px; border-radius: 9px; background: var(--primary-alpha-10); color: var(--sb-brand); }
.search-hit-body { flex: 1; min-width: 0; }
.search-hit-heading { display: flex; align-items: center; gap: 12px; }
.search-hit-title { flex: 1; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; font-size: 14px; font-weight: 600; }
.search-hit-heading time { flex-shrink: 0; color: var(--text-muted); font-size: 11px; }
.search-hit-meta { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-top: 5px; color: var(--text-muted); font-size: 11px; }
.search-hit-kind { color: var(--text-secondary); }
.search-snippet { display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; margin-top: 8px; color: var(--text-secondary); font-size: 13px; line-height: 1.7; overflow-wrap: anywhere; }
mark { color: var(--text-primary); background: var(--primary-alpha-20); border-radius: 3px; padding: 0 1px; }
.search-hit-arrow { flex-shrink: 0; margin-top: 7px; color: var(--text-muted); opacity: 0; transition: opacity .15s; }
.search-hit:hover .search-hit-arrow, .search-hit:focus-visible .search-hit-arrow { opacity: 1; }
.search-status { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 10px; height: 100%; text-align: center; color: var(--text-muted); font-size: 13px; padding: 24px; }
.search-status strong { color: var(--text-secondary); font-size: 14px; font-weight: 500; }
.search-empty-icon { display: flex; align-items: center; justify-content: center; width: 60px; height: 60px; margin-bottom: 4px; border-radius: 18px; color: var(--sb-brand); background: var(--primary-alpha-10); }
.search-error button, .search-load-more { color: var(--sb-brand); padding: 10px; cursor: pointer; font-size: 13px; }
.search-load-more { width: 100%; border-radius: 8px; }
.search-footer { display: flex; justify-content: space-between; gap: 12px; margin-top: 12px; padding-top: 12px; border-top: 1px solid var(--input-border); color: var(--text-muted); font-size: 11px; }
kbd { padding: 2px 5px; border: 1px solid var(--input-border); border-radius: 4px; font: inherit; }
.search-spinner { width: 16px; height: 16px; flex-shrink: 0; border: 2px solid var(--primary-alpha-20); border-top-color: var(--sb-brand); border-radius: 50%; animation: spin .7s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .search-spinner { animation: none; } }
@media (max-width: 480px) { .search-hit { gap: 9px; padding: 12px 6px; } .search-hit-arrow { display: none; } .search-scopes button { padding-inline: 9px; } .search-footer { font-size: 10px; } }
</style>
