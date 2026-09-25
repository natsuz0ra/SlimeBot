const { readFileSync, writeFileSync, mkdirSync, mkdtempSync, rmSync } = require('node:fs')
const { execFileSync } = require('node:child_process')
const { tmpdir } = require('node:os')
const path = require('node:path')
const { Resvg } = require('@resvg/resvg-js')

const root = path.resolve(__dirname, '../..')
const output = path.resolve(__dirname, '../resources')
const appSvg = readFileSync(path.join(root, 'frontend/public/slime-icon.svg'))
mkdirSync(output, { recursive: true })

function render(svg, size) {
  return new Resvg(svg, { fitTo: { mode: 'width', value: size } }).render().asPng()
}

writeFileSync(path.join(output, 'icon.png'), render(appSvg, 1024))
writeFileSync(path.join(output, 'tray.png'), render(appSvg, 32))

const sizes = [16, 24, 32, 48, 64, 256]
const images = sizes.map(size => render(appSvg, size))
const header = Buffer.alloc(6 + sizes.length * 16)
header.writeUInt16LE(1, 2)
header.writeUInt16LE(sizes.length, 4)
let offset = header.length
for (let i = 0; i < sizes.length; i++) {
  const entry = 6 + i * 16
  header.writeUInt8(sizes[i] === 256 ? 0 : sizes[i], entry)
  header.writeUInt8(sizes[i] === 256 ? 0 : sizes[i], entry + 1)
  header.writeUInt16LE(1, entry + 4)
  header.writeUInt16LE(32, entry + 6)
  header.writeUInt32LE(images[i].length, entry + 8)
  header.writeUInt32LE(offset, entry + 12)
  offset += images[i].length
}
writeFileSync(path.join(output, 'icon.ico'), Buffer.concat([header, ...images]))

if (process.platform === 'darwin') {
  const temp = mkdtempSync(path.join(tmpdir(), 'slimebot-icon-'))
  const iconset = path.join(temp, 'SlimeBot.iconset')
  mkdirSync(iconset)
  for (const size of [16, 32, 128, 256, 512]) {
    writeFileSync(path.join(iconset, `icon_${size}x${size}.png`), render(appSvg, size))
    writeFileSync(path.join(iconset, `icon_${size}x${size}@2x.png`), render(appSvg, size * 2))
  }
  execFileSync('iconutil', ['-c', 'icns', '-o', path.join(output, 'icon.icns'), iconset])
  rmSync(temp, { recursive: true, force: true })
}
