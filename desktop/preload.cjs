const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('slimebotDesktop', Object.freeze({
  updateCheck: () => ipcRenderer.invoke('desktop:update-check'),
  updateJob: () => ipcRenderer.invoke('desktop:update-job'),
  updateApply: () => ipcRenderer.invoke('desktop:update-apply'),
  chooseWorkingDirectory: currentPath => ipcRenderer.invoke('desktop:choose-working-directory', currentPath),
}))
