const { copyFileSync, chmodSync, existsSync, mkdirSync, mkdtempSync, readdirSync, rmSync } = require('node:fs')
const { execFileSync } = require('node:child_process')
const { tmpdir } = require('node:os')
const path = require('node:path')

const version = '15.1.0'
const archiveNames = {
  'darwin-amd64': `ripgrep-${version}-x86_64-apple-darwin.tar.gz`,
  'darwin-arm64': `ripgrep-${version}-aarch64-apple-darwin.tar.gz`,
  'windows-amd64': `ripgrep-${version}-x86_64-pc-windows-msvc.zip`,
  'linux-amd64': `ripgrep-${version}-x86_64-unknown-linux-musl.tar.gz`,
  'linux-arm64': `ripgrep-${version}-aarch64-unknown-linux-gnu.tar.gz`,
}

function findFile(directory, name) {
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const full = path.join(directory, entry.name)
    if (entry.isFile() && entry.name === name) return full
    if (entry.isDirectory()) {
      const found = findFile(full, name)
      if (found) return found
    }
  }
  return ''
}

module.exports = function prepareRipgrep(output, goos, goarch) {
  const target = `${goos}-${goarch}`
  const archiveName = archiveNames[target]
  if (!archiveName) throw new Error(`Unsupported ripgrep target: ${target}`)
  const name = goos === 'windows' ? 'rg.exe' : 'rg'
  const destination = path.join(output, 'vendor', 'ripgrep', target, name)
  if (existsSync(destination)) return
  const temp = mkdtempSync(path.join(tmpdir(), 'slimebot-rg-'))
  try {
    const archive = path.join(temp, archiveName)
    execFileSync('curl', ['-fL', '--retry', '3', '-o', archive, `https://github.com/BurntSushi/ripgrep/releases/download/${version}/${archiveName}`], { stdio: 'inherit' })
    execFileSync('tar', ['-xf', archive, '-C', temp], { stdio: 'inherit' })
    const found = findFile(temp, name)
    if (!found) throw new Error(`${name} was not found in ${archiveName}`)
    mkdirSync(path.dirname(destination), { recursive: true })
    copyFileSync(found, destination)
    if (goos !== 'windows') chmodSync(destination, 0o755)
  } finally {
    rmSync(temp, { recursive: true, force: true })
  }
}
