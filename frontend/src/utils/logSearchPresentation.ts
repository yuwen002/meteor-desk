export interface HighlightSegment {
  text: string
  matched: boolean
}

/** Split text into safe renderable segments; never inject log content as HTML. */
export function splitHighlightedText(
  text: string,
  query: string,
  useRegex = false,
  caseSensitive = false,
): HighlightSegment[] {
  if (!query) return [{ text, matched: false }]

  let pattern: RegExp
  try {
    pattern = new RegExp(useRegex ? query : query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'giu'.replace('u', caseSensitive ? 'g' : 'gi'))
  } catch {
    return [{ text, matched: false }]
  }

  const segments: HighlightSegment[] = []
  let cursor = 0
  for (const match of text.matchAll(pattern)) {
    const start = match.index ?? 0
    const value = match[0]
    if (!value) continue
    if (start > cursor) segments.push({ text: text.slice(cursor, start), matched: false })
    segments.push({ text: value, matched: true })
    cursor = start + value.length
  }
  if (cursor < text.length) segments.push({ text: text.slice(cursor), matched: false })
  return segments.length ? segments : [{ text, matched: false }]
}

/** Exclude a result when any comma/newline-separated exclusion term is present. */
export function excludeLogResults<T extends { content: string }>(items: T[], excludedTerms: string, caseSensitive = false): T[] {
  const terms = excludedTerms.split(/[\n,]/).map((term) => term.trim()).filter(Boolean)
  if (!terms.length) return items
  return items.filter((item) => {
    const content = caseSensitive ? item.content : item.content.toLocaleLowerCase()
    return !terms.some((term) => content.includes(caseSensitive ? term : term.toLocaleLowerCase()))
  })
}
