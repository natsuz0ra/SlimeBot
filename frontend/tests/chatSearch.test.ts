import assert from 'node:assert/strict'
import { test } from 'node:test'
import { highlightSearchText } from '../src/utils/chatSearch'

test('search highlighting treats special characters and HTML literally', () => {
  for (const query of ['部署', 'a+b', '[x]', '100%', 'a_b', 'C:\\tmp', '.*', '<script>']) {
    const text = `🌟 ${query} before ${query} after`
    const parts = highlightSearchText(text, query)
    assert.equal(parts.map((part) => part.text).join(''), text)
    assert.equal(parts.filter((part) => part.match).length, 2)
    assert.ok(parts.filter((part) => part.match).every((part) => part.text === query))
  }
  assert.deepEqual(highlightSearchText('KEYWORD keyword', ' keyword ').filter((part) => part.match).map((part) => part.text), ['KEYWORD', 'keyword'])
  assert.deepEqual(highlightSearchText('hello', '  '), [{ text: 'hello', match: false }])
})
