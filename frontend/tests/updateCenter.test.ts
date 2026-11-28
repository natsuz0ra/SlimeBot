import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import test from 'node:test'
import {
  isUpdateJobActive,
  normalizeUpdateCheck,
  normalizeUpdateJob,
  updateJobProgressPercent,
  updateJobStatusLabelKey,
  updatePhaseTone,
} from '../src/utils/updateStatus'
import { renderMarkdown } from '../src/utils/markdown'

test('normalizeUpdateCheck fills stable defaults', () => {
  const got = normalizeUpdateCheck({
    current: 'v1.26.1',
    latest: 'v1.26.2',
    updateAvailable: true,
    canApply: true,
  })

  assert.equal(got.current, 'v1.26.1')
  assert.equal(got.latest, 'v1.26.2')
  assert.equal(got.releaseNotes, '')
  assert.equal(got.manualHint, '')
})

test('update phase helpers classify active and terminal states', () => {
  assert.equal(isUpdateJobActive('downloading'), true)
  assert.equal(isUpdateJobActive('succeeded'), false)
  assert.equal(updatePhaseTone('failed'), 'danger')
  assert.equal(updatePhaseTone('succeeded'), 'success')
})

test('normalizeUpdateJob keeps download progress fields', () => {
  const got = normalizeUpdateJob({
    phase: 'downloading',
    downloadedBytes: 512,
    totalBytes: 1024,
    progressPercent: 50,
  })

  assert.equal(got.downloadedBytes, 512)
  assert.equal(got.totalBytes, 1024)
  assert.equal(got.progressPercent, 50)
})

test('update job status label keys keep backend messages out of localized UI', () => {
  assert.equal(updateJobStatusLabelKey('checking'), 'updateChecking')
  assert.equal(updateJobStatusLabelKey('downloading'), 'updateDownloading')
  assert.equal(updateJobStatusLabelKey('installing'), 'updateInstalling')
  assert.equal(updateJobStatusLabelKey('restarting'), 'updateRestarting')
  assert.equal(updateJobStatusLabelKey('succeeded'), 'updateSucceeded')
  assert.equal(updateJobStatusLabelKey('failed'), 'updateFailed')
  assert.equal(updateJobStatusLabelKey('idle'), '')
})

test('update job progress percent is phase aware after download completes', () => {
  assert.equal(updateJobProgressPercent(normalizeUpdateJob({
    phase: 'downloading',
    totalBytes: 100,
    downloadedBytes: 60,
    progressPercent: 60,
  })), 60)
  assert.equal(updateJobProgressPercent(normalizeUpdateJob({ phase: 'downloading' })), 42)
  assert.equal(updateJobProgressPercent(normalizeUpdateJob({ phase: 'checking' })), 0)
  assert.equal(updateJobProgressPercent(normalizeUpdateJob({ phase: 'installing' })), 100)
  assert.equal(updateJobProgressPercent(normalizeUpdateJob({ phase: 'restarting' })), 100)
  assert.equal(updateJobProgressPercent(normalizeUpdateJob({ phase: 'succeeded' })), 100)
  assert.equal(updateJobProgressPercent(normalizeUpdateJob({ phase: 'failed' })), 100)
})

test('update settings tab renders update center hooks', () => {
  const source = readFileSync(resolve(import.meta.dirname, '../src/components/settings/SettingsUpdateTab.vue'), 'utf8')

  assert.match(source, /updateAPI/)
  assert.match(source, /checkUpdate/)
  assert.match(source, /applyUpdate/)
  assert.match(source, /runPrimaryUpdateAction/)
  assert.match(source, /update-progress/)
  assert.match(source, /progressPercent/)
  assert.match(source, /manualHint/)
  assert.match(source, /renderMarkdown\(notes\)/)
  assert.match(source, /v-html="releaseNotes"/)
  assert.doesNotMatch(source, /<pre v-if="releaseNotes"/)
  assert.doesNotMatch(source, /t\('updateCenterTitle'\)/)
  assert.doesNotMatch(source, /confirmUpdate/)
  assert.doesNotMatch(source, /update-confirm/)
})

test('update settings tab uses focused update hero layout', () => {
  const source = readFileSync(resolve(import.meta.dirname, '../src/components/settings/SettingsUpdateTab.vue'), 'utf8')

  assert.match(source, /heroTitle/)
  assert.match(source, /heroDescription/)
  assert.match(source, /primaryActionLabel/)
  assert.match(source, /primaryActionIcon/)
  assert.match(source, /showManualCommand/)
  assert.match(source, /update-hero-card/)
  assert.match(source, /update-hero-main/)
  assert.match(source, /update-info-grid/)
  assert.match(source, /update-info-tile/)
  assert.match(source, /update-notes-panel/)
  assert.doesNotMatch(source, /settings-card update-card/)
  assert.doesNotMatch(source, /class="update-meta"/)
})

test('update settings tab renders succeeded progress as complete', () => {
  const source = readFileSync(resolve(import.meta.dirname, '../src/components/settings/SettingsUpdateTab.vue'), 'utf8')

  assert.match(source, /updateJobProgressPercent/)
  assert.match(source, /progressPercent = computed\(\(\) => updateJobProgressPercent\(job\.value\)\)/)
  assert.match(source, /progressStyle[\s\S]*progressPercent\.value/)
  assert.doesNotMatch(source, /width: `\$\{job\.value\.totalBytes > 0 \? progressPercent\.value : 42\}%`/)
})

test('update settings tab treats terminal jobs as history when a new update is available', () => {
  const source = readFileSync(resolve(import.meta.dirname, '../src/components/settings/SettingsUpdateTab.vue'), 'utf8')

  assert.match(source, /const hasActiveUpdateJob = computed/)
  assert.match(source, /const shouldShowTerminalJob = computed/)
  assert.match(source, /statusText[\s\S]*updateJobStatusLabelKey\(job\.value\.phase\)/)
  assert.doesNotMatch(source, /statusText[\s\S]*hasActiveUpdateJob\.value && job\.value\.message[\s\S]*return job\.value\.message/)
  assert.doesNotMatch(source, /return job\.value\.message \|\| ''/)
  assert.match(source, /canApplyUpdate[\s\S]*!hasActiveUpdateJob\.value/)
  assert.match(source, /showProgress[\s\S]*hasActiveUpdateJob\.value[\s\S]*shouldShowTerminalJob\.value/)
  assert.doesNotMatch(source, /showApplyAction = computed\(\(\) => hasUpdate\.value && job\.value\.phase !== 'succeeded' && job\.value\.phase !== 'failed'\)/)
})

test('update settings tab resumes polling when an active job already exists', () => {
  const source = readFileSync(resolve(import.meta.dirname, '../src/components/settings/SettingsUpdateTab.vue'), 'utf8')

  assert.match(source, /async function loadJob\([^)]*pollIfActive/)
  assert.match(source, /if \(applying\.value && pollIfActive\)[\s\S]*startPolling\(\)/)
  assert.match(source, /await loadJob\(true\)/)
})

test('update settings tab uses one primary update action without duplicate progress text', () => {
  const source = readFileSync(resolve(import.meta.dirname, '../src/components/settings/SettingsUpdateTab.vue'), 'utf8')

  assert.match(source, /const primaryActionIcon = computed/)
  assert.match(source, /runPrimaryUpdateAction[\s\S]*if \(hasUpdate\.value\)[\s\S]*await applyUpdate\(\)[\s\S]*await checkUpdate\(true\)/)
  assert.match(source, /@click="runPrimaryUpdateAction"/)
  assert.doesNotMatch(source, /@click="checkUpdate\(true\)"/)
  assert.doesNotMatch(source, /v-if="showApplyAction"/)
  assert.doesNotMatch(source, /applying\.value \|\| shouldShowTerminalJob\.value\) && progressLabel\.value/)
})

test('about settings tab keeps only static about content', () => {
  const source = readFileSync(resolve(import.meta.dirname, '../src/components/settings/SettingsAboutTab.vue'), 'utf8')

  assert.match(source, /SlimeBot/)
  assert.match(source, /githubRepository/)
  assert.doesNotMatch(source, /updateAPI/)
  assert.doesNotMatch(source, /updateCenterTitle/)
  assert.doesNotMatch(source, /checkUpdate/)
})

test('home and settings surfaces render unread update notice dots', () => {
  const homeSource = readFileSync(resolve(import.meta.dirname, '../src/pages/HomePage.vue'), 'utf8')
  const sidebarSource = readFileSync(resolve(import.meta.dirname, '../src/components/home/HomeSidebar.vue'), 'utf8')
  const dialogsSource = readFileSync(resolve(import.meta.dirname, '../src/components/home/HomeDialogs.vue'), 'utf8')
  const settingsSource = readFileSync(resolve(import.meta.dirname, '../src/components/settings/SettingsPanel.vue'), 'utf8')

  assert.match(homeSource, /useUpdateNotice\(\)/)
  assert.match(homeSource, /checkForUpdate\(\)/)
  assert.match(homeSource, /:has-update-notice="hasUnreadUpdate"/)
  assert.match(homeSource, /:mark-update-notice-read="markUpdateNoticeRead"/)
  assert.match(sidebarSource, /hasUpdateNotice:\s*boolean/)
  assert.match(sidebarSource, /v-if="hasUpdateNotice"[\s\S]*update-notice-dot/)
  assert.match(dialogsSource, /hasUpdateNotice:\s*boolean/)
  assert.match(dialogsSource, /markUpdateNoticeRead:\s*\(\) => void/)
  assert.match(settingsSource, /hasUpdateNotice:\s*boolean/)
  assert.match(settingsSource, /markUpdateNoticeRead:\s*\(\) => void/)
  assert.match(settingsSource, /SettingsUpdateTab/)
  assert.match(settingsSource, /\{\s*key:\s*'update',\s*labelKey:\s*'updateSettings'[^}]*\}[\s\S]*\{\s*key:\s*'about',\s*labelKey:\s*'aboutSettings'[^}]*\}/)
  assert.match(settingsSource, /<span[^>]*settings-tab-label[^>]*>\{\{ t\(item\.labelKey\) \}\}<\/span>[\s\S]*item\.key === 'update' && hasUpdateNotice[\s\S]*update-notice-dot/)
  assert.match(settingsSource, /watch\(tab[\s\S]*nextTab === 'update'[\s\S]*markUpdateNoticeRead\(\)/)
})

test('release notes markdown renders as html instead of raw markdown', () => {
  const html = renderMarkdown('## Highlights\n- **A** item\n- [Release](https://example.test/release)')

  assert.match(html, /<h2>Highlights<\/h2>/)
  assert.match(html, /<strong>A<\/strong> item/)
  assert.match(html, /target="_blank"/)
  assert.doesNotMatch(html, /## Highlights/)
  assert.doesNotMatch(html, /\*\*A\*\*/)
})

test('i18n contains update center labels in both languages', () => {
  const source = readFileSync(resolve(import.meta.dirname, '../src/i18n.ts'), 'utf8')

  assert.match(source, /updateSettings: '更新'/)
  assert.match(source, /updateSettings: 'Update'/)
  assert.match(source, /downloadUpdate: '下载更新'/)
  assert.match(source, /downloadUpdate: 'Download Update'/)
  assert.match(source, /updateDownloading: '下载中'/)
  assert.match(source, /updateDownloading: 'Downloading'/)
  assert.match(source, /updateRestarting/)
  assert.match(source, /updateManualHint/)
  assert.match(source, /updateConfirmTitle/)
  assert.match(source, /updateDownloadProgress/)
})
