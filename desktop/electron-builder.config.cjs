const path = require('node:path')

const windows = process.platform === 'win32'

module.exports = {
  appId: 'com.natsuzora.slimebot.desktop',
  productName: 'SlimeBot',
  extraMetadata: { slimebotSigned: process.platform === 'win32' && Boolean(process.env.CSC_LINK) },
  directories: { output: 'dist' },
  files: ['main.cjs', 'preload.cjs', 'package.json', 'resources/icon.png', 'resources/tray.png'],
  extraResources: [
    {
      from: path.join('build', windows ? 'slimebot.exe' : 'slimebot'),
      to: path.join('bin', windows ? 'slimebot.exe' : 'slimebot'),
    },
    { from: 'build/vendor', to: 'bin/vendor' },
  ],
  asar: true,
  mac: {
    category: 'public.app-category.productivity',
    icon: 'resources/icon.icns',
    artifactName: 'SlimeBot-${version}-macos-${arch}.${ext}',
    hardenedRuntime: true,
    x64ArchFiles: 'Contents/Resources/bin/vendor/ripgrep/**',
    target: ['dmg'],
  },
  win: {
    icon: 'resources/icon.ico',
    artifactName: 'SlimeBot-${version}-windows-${arch}.${ext}',
    target: ['nsis'],
  },
  nsis: {
    oneClick: false,
    perMachine: false,
    allowToChangeInstallationDirectory: true,
  },
  publish: { provider: 'github', owner: 'natsuz0ra', repo: 'SlimeBot' },
}
