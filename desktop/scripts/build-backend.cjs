const { execFileSync } = require('node:child_process')
const { mkdirSync, rmSync } = require('node:fs')
const path = require('node:path')

const root = path.resolve(__dirname, '../..')
const desktop = path.resolve(__dirname, '..')
const output = path.join(desktop, 'build')
const version = require('../package.json').version
const prepareRipgrep = require('./prepare-ripgrep.cjs')

function run(command, args, options = {}) {
  execFileSync(command, args, { cwd: root, stdio: 'inherit', ...options })
}

mkdirSync(output, { recursive: true })
if (!process.env.npm_execpath) throw new Error('Run this script through npm')
run(process.execPath, [process.env.npm_execpath, '--prefix', 'frontend', 'run', 'build'])

function build(goos, goarch, dest) {
  rmSync(dest, { force: true })
  run('go', ['build', '-trimpath', '-ldflags', `-s -w -X slimebot/internal/version.Version=v${version}`, '-o', dest, './cmd/server'], {
    env: { ...process.env, GOOS: goos, GOARCH: goarch, CGO_ENABLED: '0' },
  })
}

if (process.platform === 'darwin' && process.env.SLIMEBOT_DESKTOP_UNIVERSAL === '1') {
  const arm64 = path.join(output, 'slimebot-arm64')
  const amd64 = path.join(output, 'slimebot-amd64')
  build('darwin', 'arm64', arm64)
  build('darwin', 'amd64', amd64)
  const universal = path.join(output, 'slimebot')
  rmSync(universal, { force: true })
  run('lipo', ['-create', '-output', universal, arm64, amd64])
  prepareRipgrep(output, 'darwin', 'arm64')
  prepareRipgrep(output, 'darwin', 'amd64')
} else {
  const goos = process.platform === 'win32' ? 'windows' : process.platform === 'darwin' ? 'darwin' : 'linux'
  const goarch = process.arch === 'arm64' ? 'arm64' : 'amd64'
  build(goos, goarch, path.join(output, process.platform === 'win32' ? 'slimebot.exe' : 'slimebot'))
  prepareRipgrep(output, goos, goarch)
}
