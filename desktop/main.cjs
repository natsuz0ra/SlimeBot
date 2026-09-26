const { app, BrowserWindow, Tray, Menu, dialog, ipcMain, nativeImage, shell, session } = require('electron')
const { autoUpdater } = require('electron-updater')
const { spawn } = require('node:child_process')
const { randomBytes } = require('node:crypto')
const path = require('node:path')
const { mkdirSync } = require('node:fs')

if (!app.isPackaged && process.env.SLIMEBOT_DESKTOP_USER_DATA_DIR) {
  mkdirSync(process.env.SLIMEBOT_DESKTOP_USER_DATA_DIR, { recursive: true })
  app.setPath('userData', process.env.SLIMEBOT_DESKTOP_USER_DATA_DIR)
}
const signedBuild = Boolean(require('./package.json').slimebotSigned)

let window
let tray
let backend
let backendToken = ''
let backendOrigin = ''
let quitting = false
let quitPending = false
let stoppingBackend = false
let updateInfo = null
let updateAvailable = false
let updateError = ''
let job = emptyJob()

let chinese = false
const label = (zh, en) => chinese ? zh : en

function emptyJob() {
  return {
    phase: 'idle', current: `v${app.getVersion()}`, target: '', message: '', error: '',
    manualHint: '', downloadedBytes: 0, totalBytes: 0, progressPercent: 0,
    updatedAt: new Date().toISOString(),
  }
}

function setJob(values) {
  job = { ...job, ...values, updatedAt: new Date().toISOString() }
}

function backendExecutable() {
  if (process.env.SLIMEBOT_DESKTOP_BINARY && !app.isPackaged) return process.env.SLIMEBOT_DESKTOP_BINARY
  return app.isPackaged
    ? path.join(process.resourcesPath, 'bin', process.platform === 'win32' ? 'slimebot.exe' : 'slimebot')
    : path.join(__dirname, 'build', process.platform === 'win32' ? 'slimebot.exe' : 'slimebot')
}

function startBackend() {
  return new Promise((resolve, reject) => {
    const token = randomBytes(32).toString('hex')
    const backendEnv = { ...process.env }
    for (const key of ['SERVER_PORT', 'FRONTEND_ORIGIN', 'JWT_SECRET']) {
      delete backendEnv[key]
    }
    const child = spawn(backendExecutable(), ['desktop-host'], {
      cwd: app.getPath('home'),
      env: backendEnv,
      stdio: ['pipe', 'pipe', 'pipe'],
      windowsHide: true,
    })
    backend = child
    let settled = false
    let output = ''
    let errors = ''
    const timer = setTimeout(() => fail(new Error(label('后端启动超时', 'Backend startup timed out'))), 30000)

    function fail(error) {
      if (settled) return
      settled = true
      clearTimeout(timer)
      child.kill()
      reject(new Error(`${error.message}${errors ? `\n${errors.slice(-2000)}` : ''}`))
    }

    child.once('error', fail)
    child.stdin.on('error', fail)
    child.on('exit', (code, signal) => {
      if (!settled) {
        fail(new Error(`Backend exited before ready (${code ?? signal})`))
      } else if (backend === child && !stoppingBackend && !quitting) {
        backend = null
        backendOrigin = ''
        void recover(new Error(`Backend exited (${code ?? signal})`))
      }
    })
    child.stderr.on('data', chunk => { errors = (errors + chunk.toString()).slice(-8000) })
    child.stdout.on('data', chunk => {
      if (settled) return
      output += chunk.toString()
      if (output.length > 8192) return fail(new Error('Invalid backend ready message'))
      const newline = output.indexOf('\n')
      if (newline < 0) return
      try {
        const ready = JSON.parse(output.slice(0, newline))
        if (ready.version !== '1' || !/^127\.0\.0\.1:\d+$/.test(ready.address)) {
          throw new Error('Invalid backend address or protocol')
        }
        backendToken = token
        backendOrigin = `http://${ready.address}`
        settled = true
        clearTimeout(timer)
        resolve(backendOrigin)
      } catch (error) {
        fail(error)
      }
    })
    child.stdin.write(`${JSON.stringify({ token })}\n`)
  })
}

async function stopBackend() {
  const child = backend
  if (!child || child.exitCode !== null) return
  stoppingBackend = true
  child.stdin.end()
  await Promise.race([
    new Promise(resolve => child.once('exit', resolve)),
    new Promise(resolve => setTimeout(resolve, 6000)),
  ])
  if (child.exitCode === null) child.kill()
  backend = null
  backendOrigin = ''
  backendToken = ''
}

async function recover(error) {
  if (quitting) return
  window?.hide()
  const result = await dialog.showMessageBox({
    type: 'error', title: 'SlimeBot', message: label('桌面服务已停止', 'Desktop service stopped'),
    detail: error.message,
    buttons: [label('重试', 'Retry'), label('退出', 'Quit')],
    defaultId: 0, cancelId: 1,
  })
  if (result.response !== 0) return requestQuit(true)
  try {
    const origin = await startBackend()
    if (window) {
      await window.loadURL(origin)
      showWindow()
    } else {
      makeWindow(origin)
    }
  } catch (retryError) {
    await recover(retryError)
  }
}

function showWindow() {
  if (!window) return
  if (window.isMinimized()) window.restore()
  window.show()
  window.focus()
}

function makeWindow(origin) {
  window = new BrowserWindow({
    width: 1280, height: 850, minWidth: 900, minHeight: 620,
    title: 'SlimeBot',
    icon: path.join(__dirname, 'resources', 'icon.png'),
    show: false,
    webPreferences: {
      preload: path.join(__dirname, 'preload.cjs'),
      nodeIntegration: false,
      contextIsolation: true,
      sandbox: true,
    },
  })
  window.once('ready-to-show', showWindow)
  window.on('close', event => {
    if (!quitting) {
      event.preventDefault()
      window.hide()
    }
  })
  window.webContents.on('will-navigate', (event, url) => {
    if (!url.startsWith(`${backendOrigin}/`)) event.preventDefault()
  })
  window.webContents.setWindowOpenHandler(({ url }) => {
    if (url.startsWith('https://')) void shell.openExternal(url)
    return { action: 'deny' }
  })
  void window.loadURL(origin)
}

function makeTray() {
  tray = new Tray(nativeImage.createFromPath(path.join(__dirname, 'resources', 'tray.png')))
  tray.setToolTip('SlimeBot')
  tray.setContextMenu(Menu.buildFromTemplate([
    { label: label('打开 SlimeBot', 'Open SlimeBot'), click: showWindow },
    { type: 'separator' },
    { label: label('退出 SlimeBot', 'Quit SlimeBot'), click: () => { void requestQuit() } },
  ]))
  tray.on('click', showWindow)
}

async function requestQuit(force = false) {
  if (quitPending || quitting) return
  quitPending = true
  if (!force && backend) {
    const result = await dialog.showMessageBox({
      type: 'question', title: 'SlimeBot',
      message: label('退出 SlimeBot？', 'Quit SlimeBot?'),
      detail: label('消息平台和后台任务会停止。关闭窗口可让它们继续运行。', 'Message platforms and background tasks will stop. Close the window to keep them running.'),
      buttons: [label('取消', 'Cancel'), label('退出', 'Quit')],
      defaultId: 1, cancelId: 0,
    })
    if (result.response !== 1) {
      quitPending = false
      return
    }
  }
  quitting = true
  await stopBackend()
  app.quit()
}

function registerSecurity() {
  const webSession = session.defaultSession
  webSession.setPermissionRequestHandler((_contents, _permission, callback) => callback(false))
  webSession.webRequest.onBeforeSendHeaders((details, callback) => {
    if (backendOrigin && backendToken &&
      (details.url.startsWith(`${backendOrigin}/api/`) || details.url.startsWith(backendOrigin.replace('http:', 'ws:') + '/ws/'))) {
      details.requestHeaders.Authorization = `Bearer ${backendToken}`
    }
    callback({ requestHeaders: details.requestHeaders })
  })
}

function registerUpdates() {
  autoUpdater.autoDownload = false
  autoUpdater.autoInstallOnAppQuit = false
  autoUpdater.on('update-available', info => {
    updateInfo = info
    updateAvailable = true
  })
  autoUpdater.on('update-not-available', info => {
    updateInfo = info
    updateAvailable = false
  })
  autoUpdater.on('download-progress', progress => {
    setJob({ phase: 'downloading', downloadedBytes: progress.transferred, totalBytes: progress.total, progressPercent: Math.trunc(progress.percent) })
  })
  autoUpdater.on('update-downloaded', () => setJob({ phase: 'ready' }))
  autoUpdater.on('error', error => {
    updateError = error.message
    setJob({ phase: 'failed', error: error.message })
  })

  ipcMain.handle('desktop:update-check', async event => {
    verifySender(event)
    if (!signedBuild) return checkPreviewUpdate()
    updateError = ''
    const result = await autoUpdater.checkForUpdates()
    const info = result?.updateInfo || updateInfo
    const latest = info?.version ? `v${info.version}` : `v${app.getVersion()}`
    return {
      current: `v${app.getVersion()}`, latest, updateAvailable,
      canApply: updateAvailable, reason: updateError,
      releaseName: latest,
      releaseNotes: typeof info?.releaseNotes === 'string' ? info.releaseNotes : '',
      releaseUrl: `https://github.com/natsuz0ra/SlimeBot/releases/tag/${latest}`,
      publishedAt: info?.releaseDate || '', assetName: '', manualHint: '',
    }
  })
  ipcMain.handle('desktop:update-job', event => { verifySender(event); return job })
  ipcMain.handle('desktop:update-apply', async event => {
    verifySender(event)
    if (!signedBuild) throw new Error(label('预览版请从 Release 页面下载安装包', 'Download the preview installer from Releases'))
    if (job.phase === 'ready') {
      const result = await dialog.showMessageBox(window, {
        type: 'question', title: 'SlimeBot',
        message: label('安装更新并重启？', 'Install update and restart?'),
        detail: label('消息平台和后台任务会在重启期间中断。', 'Message platforms and background tasks will pause during restart.'),
        buttons: [label('取消', 'Cancel'), label('安装并重启', 'Install and Restart')],
        defaultId: 1, cancelId: 0,
      })
      if (result.response === 1) {
        setJob({ phase: 'installing' })
        quitting = true
        await stopBackend()
        setTimeout(() => autoUpdater.quitAndInstall(false, true), 100)
      }
      return job
    }
    if (!updateAvailable) throw new Error(label('没有可安装的更新', 'No update is available'))
    setJob({ phase: 'downloading', target: `v${updateInfo.version}`, error: '' })
    await autoUpdater.downloadUpdate()
    return job
  })
}

async function checkPreviewUpdate() {
  const response = await fetch('https://api.github.com/repos/natsuz0ra/SlimeBot/releases/latest', {
    headers: { 'User-Agent': 'SlimeBot-Desktop' },
  })
  if (!response.ok) throw new Error(`GitHub Releases: ${response.status}`)
  const release = await response.json()
  const latest = String(release.tag_name || '')
  const current = `v${app.getVersion()}`
  const versionParts = value => /^v?(\d+)\.(\d+)\.(\d+)$/.exec(value)?.slice(1).map(Number)
  const left = versionParts(latest)
  const right = versionParts(current)
  const hasUpdate = Boolean(left && right && left.some((part, index) => part > right[index] && left.slice(0, index).every((value, i) => value === right[i])))
  const reason = label('未签名预览版：请从 Release 页面下载安装包', 'Unsigned preview: download the installer from Releases')
  return {
    current, latest, updateAvailable: hasUpdate, canApply: false,
    reason: hasUpdate ? reason : '', releaseName: release.name || latest,
    releaseNotes: hasUpdate ? release.body || '' : '', releaseUrl: hasUpdate ? release.html_url || '' : '',
    publishedAt: release.published_at || '', assetName: '', manualHint: hasUpdate ? reason : '',
  }
}

function verifySender(event) {
  if (!window || event.sender !== window.webContents || !event.senderFrame.url.startsWith(`${backendOrigin}/`)) {
    throw new Error('Untrusted desktop request')
  }
}

if (!app.requestSingleInstanceLock()) {
  app.quit()
} else {
  app.on('second-instance', showWindow)
  app.on('activate', showWindow)
  app.on('window-all-closed', () => {})
  app.on('before-quit', event => {
    if (!quitting) {
      event.preventDefault()
      void requestQuit()
    }
  })
  app.whenReady().then(async () => {
    chinese = app.getLocale().toLowerCase().startsWith('zh')
    if (process.platform === 'win32') app.setAppUserModelId('com.natsuzora.slimebot.desktop')
    registerSecurity()
    registerUpdates()
    makeTray()
    Menu.setApplicationMenu(Menu.buildFromTemplate([
      { label: 'SlimeBot', submenu: [
        { label: label('打开', 'Open'), click: showWindow },
        { type: 'separator' },
        { label: label('退出', 'Quit'), accelerator: 'CommandOrControl+Q', click: () => { void requestQuit() } },
      ] },
      { role: 'editMenu' },
    ]))
    try {
      const origin = await startBackend()
      makeWindow(origin)
    } catch (error) {
      await recover(error)
    }
  }).catch(error => { void recover(error) })
}
