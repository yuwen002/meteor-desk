/** Convert a 1-based result number to a zero-based list index. */
export function resolveResultJumpIndex(value: string, total: number): number | null {
  if (!Number.isInteger(total) || total <= 0) return null
  const requested = Number(value.trim())
  if (!Number.isInteger(requested) || requested < 1 || requested > total) return null
  return requested - 1
}
