import test from 'node:test'
import assert from 'node:assert/strict'

import {
  extractPastedFiles,
  isImageFile,
  MAX_CHAT_UPLOAD_BYTES,
  MAX_CHAT_UPLOAD_FILES,
  mergePendingFiles,
  normalizePastedFile,
} from '../src/utils/chatAttachments'

function makeFile(name: string, size: number, type = 'text/plain'): File {
  const bytes = new Uint8Array(Math.min(size, 16))
  const file = new File([bytes], name, { type })
  if (size !== file.size) {
    Object.defineProperty(file, 'size', { value: size })
  }
  return file
}

test('normalizePastedFile renames generic screenshot names with timestamp and preserves custom names', () => {
  const now = new Date(2026, 10, 26, 9, 5, 3)
  const generic = makeFile('image.png', 100, 'image/png')
  const renamed = normalizePastedFile(generic, now, 0)
  assert.equal(renamed.name, 'screenshot-20261126-090503.png')

  const second = normalizePastedFile(makeFile('blob', 100, 'image/webp'), now, 1)
  assert.equal(second.name, 'screenshot-20261126-090503-2.webp')

  const named = makeFile('architecture-diagram.png', 100, 'image/png')
  assert.equal(normalizePastedFile(named, now, 0).name, 'architecture-diagram.png')
})

test('extractPastedFiles extracts pure screenshots and ignores rich HTML+text paste', () => {
  const now = new Date(2026, 10, 26, 12, 0, 0)
  const img = makeFile('image.png', 120, 'image/png')

  const pureScreenshot = extractPastedFiles({
    items: [{ kind: 'file', getAsFile: () => img }],
    getData: () => '',
  }, now)
  assert.equal(pureScreenshot.length, 1)
  assert.equal(pureScreenshot[0]?.name, 'screenshot-20261126-120000.png')

  const richWordCopy = extractPastedFiles({
    items: [{ kind: 'file', getAsFile: () => img }],
    getData: (format) => (format === 'text/plain' ? 'Table cell' : '<table><tr><td>Table cell</td></tr></table>'),
  }, now)
  assert.deepEqual(richWordCopy, [])
})

test('mergePendingFiles enforces 10MB file size limit and 5-file count cap with skip reasons', () => {
  const existing = [makeFile('a.txt', 10), makeFile('b.txt', 10)]
  const incoming = [
    makeFile('huge.bin', MAX_CHAT_UPLOAD_BYTES + 1, 'application/octet-stream'),
    makeFile('c.txt', 10),
    makeFile('d.txt', 10),
    makeFile('e.txt', 10),
    makeFile('f.txt', 10),
  ]

  const result = mergePendingFiles(existing, incoming)
  assert.equal(result.files.length, MAX_CHAT_UPLOAD_FILES)
  assert.equal(result.addedCount, 3)
  assert.deepEqual(result.skippedTooLarge, ['huge.bin'])
  assert.equal(result.skippedOverflow, 1)
  assert.deepEqual(result.files.map((f) => f.name), ['a.txt', 'b.txt', 'c.txt', 'd.txt', 'e.txt'])
})

test('isImageFile detects image MIME types and extensions', () => {
  assert.equal(isImageFile(makeFile('photo', 10, 'image/jpeg')), true)
  assert.equal(isImageFile(makeFile('diagram.webp', 10, '')), true)
  assert.equal(isImageFile(makeFile('notes.md', 10, 'text/markdown')), false)
})
