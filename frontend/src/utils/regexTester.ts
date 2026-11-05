// 正则表达式调试器：给定正则与测试文本，做一次匹配并返回命中段、分组与耗时。
// 所有匹配在当前进程内存中完成，不执行任意代码；正则来源是用户输入，因此包一层
// try/catch 并把异常当作可读错误返回，而不是抛给界面。

export interface RegexHit {
  start: number
  end: number
  text: string
  groups: (string | undefined)[]
  groupsStart: (number | undefined)[]
}

export interface RegexTestResult {
  error: string
  flags: string
  matchCount: number
  hits: RegexHit[]
  durationMs: number
  durationPreciseMs: number
}

export function compilePattern(pattern: string, flags: string): { regex: RegExp } | { error: string } {
  // 匹配逻辑依赖 global 逐次推进 lastIndex，这里强制补上 g。
  const normalized = flags.includes('g') ? flags : flags + 'g'
  try {
    return { regex: new RegExp(pattern, normalized) }
  } catch (error) {
    return { error: String(error) }
  }
}

export function testRegex(pattern: string, flags: string, input: string): RegexTestResult {
  const started = performance.now()
  const base: RegexTestResult = {
    error: '',
    flags,
    matchCount: 0,
    hits: [],
    durationMs: 0,
    durationPreciseMs: 0,
  }
  if (!pattern) return { ...base, error: '请输入正则表达式。' }

  const compiled = compilePattern(pattern, flags)
  if ('error' in compiled) return { ...base, error: compiled.error }
  const regex = compiled.regex

  let matchCount = 0
  const hits: RegexHit[] = []
  let match: RegExpExecArray | null
  let guard = 0
  // 用安全上限保护：即使输入超长，也最多展示前 10 万个命中，避免界面卡死。
  const maxMatches = 100_000
  while ((match = regex.exec(input)) !== null) {
    hits.push({
      start: match.index,
      end: match.index + match[0].length,
      text: match[0],
      groups: match.slice(1),
      groupsStart: match.indices?.map(pair => (pair ? pair[0] : undefined)) ?? [],
    })
    matchCount++
    if (match[0] === '') regex.lastIndex++
    if (!regex.global || matchCount >= maxMatches) break
    guard++
    if (guard > maxMatches + 10) break
  }

  const durationPreciseMs = performance.now() - started
  return {
    ...base,
    matchCount,
    hits,
    durationMs: Math.round(durationPreciseMs),
    durationPreciseMs,
  }
}