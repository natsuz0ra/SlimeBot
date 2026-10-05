/** Literal matching keeps regex characters and message HTML as plain text. */
export function highlightSearchText(text: string, query: string) {
  const needle = query.trim()
  if (!needle) return [{ text, match: false }]
  const escaped = needle.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return text.split(new RegExp(`(${escaped})`, 'giu'))
    .map((text, index) => ({ text, match: index % 2 === 1 }))
    .filter((part) => part.text !== '')
}
