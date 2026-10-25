export type DiffKind = 'equal' | 'added' | 'removed'
export interface DiffLine { kind: DiffKind; value: string }

export function diffText(before: string, after: string, maxLines = 2000): DiffLine[] {
  const left = before.split('\n'), right = after.split('\n')
  if (left.length + right.length > maxLines) throw new Error(`文本行数过多，最多比较 ${maxLines} 行。`)
  const table = Array.from({ length: left.length + 1 }, () => new Uint16Array(right.length + 1))
  for (let i = left.length - 1; i >= 0; i--) for (let j = right.length - 1; j >= 0; j--) table[i][j] = left[i] === right[j] ? table[i + 1][j + 1] + 1 : Math.max(table[i + 1][j], table[i][j + 1])
  const result: DiffLine[] = []
  let i = 0, j = 0
  while (i < left.length && j < right.length) {
    if (left[i] === right[j]) { result.push({ kind: 'equal', value: left[i++] }); j++; continue }
    if (table[i + 1][j] >= table[i][j + 1]) result.push({ kind: 'removed', value: left[i++] })
    else result.push({ kind: 'added', value: right[j++] })
  }
  while (i < left.length) result.push({ kind: 'removed', value: left[i++] })
  while (j < right.length) result.push({ kind: 'added', value: right[j++] })
  return result
}
