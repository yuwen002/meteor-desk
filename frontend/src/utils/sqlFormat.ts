const SQL_KEYWORDS = [
  'SELECT', 'FROM', 'WHERE', 'GROUP BY', 'ORDER BY', 'HAVING',
  'LEFT JOIN', 'RIGHT JOIN', 'INNER JOIN', 'OUTER JOIN', 'FULL JOIN', 'CROSS JOIN', 'JOIN',
  'ON', 'AS', 'USING',
  'INSERT INTO', 'VALUES', 'UPDATE', 'SET', 'DELETE FROM',
  'CREATE TABLE', 'DROP TABLE', 'ALTER TABLE', 'CREATE INDEX', 'DROP INDEX',
  'PRIMARY KEY', 'FOREIGN KEY', 'REFERENCES', 'UNIQUE', 'CHECK', 'CONSTRAINT', 'DEFAULT',
  'AND', 'OR', 'NOT', 'IN', 'NOT IN', 'BETWEEN', 'LIKE', 'ILIKE', 'IS NULL', 'IS NOT NULL',
  'LIMIT', 'OFFSET', 'UNION', 'UNION ALL', 'EXCEPT', 'INTERSECT',
  'CASE', 'WHEN', 'THEN', 'ELSE', 'END',
  'DISTINCT', 'ALL', 'ANY', 'EXISTS', 'WITH', 'RECURSIVE',
  'BEGIN', 'COMMIT', 'ROLLBACK', 'TRANSACTION', 'SAVEPOINT',
  'INTO', 'OVER', 'PARTITION BY', 'WINDOW',
]

const SQL_FUNCTIONS = [
  'COUNT', 'SUM', 'AVG', 'MIN', 'MAX',
  'COALESCE', 'IFNULL', 'NULLIF', 'CAST', 'CONVERT',
  'NOW', 'CURRENT_TIMESTAMP', 'CURRENT_DATE', 'CURRENT_TIME',
  'ABS', 'ROUND', 'CEIL', 'FLOOR', 'TRUNCATE',
  'LENGTH', 'UPPER', 'LOWER', 'TRIM', 'LTRIM', 'RTRIM', 'SUBSTRING', 'REPLACE', 'CONCAT',
  'ROW_NUMBER', 'RANK', 'DENSE_RANK', 'LAG', 'LEAD',
]

const ALL_KEYWORDS_SET = new Set([...SQL_KEYWORDS, ...SQL_FUNCTIONS])

export function stripSqlComments(sql: string): string {
  let out = ''
  let i = 0
  let inSingle = false
  let inDouble = false
  let inBacktick = false
  while (i < sql.length) {
    const ch = sql[i]
    const next = sql[i + 1]
    if (inSingle) {
      out += ch
      if (ch === "'" && next === "'") { out += next; i += 2; continue }
      if (ch === "'") inSingle = false
      i++; continue
    }
    if (inDouble) {
      out += ch
      if (ch === '"' && next === '"') { out += next; i += 2; continue }
      if (ch === '"') inDouble = false
      i++; continue
    }
    if (inBacktick) {
      out += ch
      if (ch === '`') inBacktick = false
      i++; continue
    }
    if (ch === "'") { inSingle = true; out += ch; i++; continue }
    if (ch === '"') { inDouble = true; out += ch; i++; continue }
    if (ch === '`') { inBacktick = true; out += ch; i++; continue }
    if (ch === '-' && next === '-') {
      while (i < sql.length && sql[i] !== '\n') i++
      continue
    }
    if (ch === '/' && next === '*') {
      i += 2
      while (i < sql.length - 1 && !(sql[i] === '*' && sql[i + 1] === '/')) i++
      i += 2
      continue
    }
    out += ch
    i++
  }
  return out
}

export function compactSql(sql: string): string {
  const withoutComments = stripSqlComments(sql)
  return withoutComments
    .replace(/\s+/g, ' ')
    .replace(/\s*([(),;])\s*/g, '$1')
    .trim()
}

export type KeywordCase = 'keep' | 'upper' | 'lower'

export function applyKeywordCase(sql: string, mode: KeywordCase): string {
  if (mode === 'keep') return sql
  const target = mode === 'upper' ? (w: string) => w.toUpperCase() : (w: string) => w.toLowerCase()
  const sorted = [...ALL_KEYWORDS_SET].sort((a, b) => b.length - a.length)
  let result = sql
  for (const kw of sorted) {
    const escaped = kw.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    const re = new RegExp('\\b' + escaped + '\\b', 'gi')
    result = result.replace(re, target)
  }
  return result
}

export function formatSql(sql: string, opts: { case?: KeywordCase; indent?: number } = {}): string {
  const { case: caseMode = 'keep', indent = 2 } = opts
  let processed = stripSqlComments(sql)
  processed = applyKeywordCase(processed, caseMode)
  processed = compactSql(processed)

  const indentStr = ' '.repeat(indent)
  const lines: string[] = []
  let current = ''
  let depth = 0
  let inSingle = false
  let inDouble = false
  let inBacktick = false
  let i = 0

  while (i < processed.length) {
    const ch = processed[i]
    const next = processed[i + 1]

    if (inSingle) { current += ch; if (ch === "'" && next !== "'") inSingle = false; i++; continue }
    if (inDouble) { current += ch; if (ch === '"') inDouble = false; i++; continue }
    if (inBacktick) { current += ch; if (ch === '`') inBacktick = false; i++; continue }

    if (ch === "'") { inSingle = true; current += ch; i++; continue }
    if (ch === '"') { inDouble = true; current += ch; i++; continue }
    if (ch === '`') { inBacktick = true; current += ch; i++; continue }

    if (ch === '(') {
      current += ch
      depth++
      i++
      continue
    }
    if (ch === ')') {
      depth = Math.max(0, depth - 1)
      current += ch
      i++
      continue
    }
    if (ch === ';') {
      current += ch
      lines.push(current.trim())
      current = ''
      depth = 0
      i++
      continue
    }

    let matchedKeyword: string | null = null
    for (const kw of SQL_KEYWORDS) {
      const upper = kw.toUpperCase()
      const len = upper.length
      const candidate = processed.substring(i, i + len).toUpperCase()
      const beforeOk = i === 0 || !/[A-Za-z0-9_]/.test(processed[i - 1])
      const afterOk = i + len >= processed.length || !/[A-Za-z0-9_]/.test(processed[i + len])
      if (candidate === upper && beforeOk && afterOk) {
        matchedKeyword = kw
        break
      }
    }

    if (matchedKeyword) {
      const k = matchedKeyword
      const needsBreakBefore = [
        'SELECT', 'FROM', 'WHERE', 'GROUP BY', 'ORDER BY', 'HAVING',
        'LEFT JOIN', 'RIGHT JOIN', 'INNER JOIN', 'OUTER JOIN', 'FULL JOIN', 'CROSS JOIN', 'JOIN',
        'INSERT INTO', 'UPDATE', 'DELETE FROM',
        'VALUES', 'SET', 'ON', 'USING', 'AS',
        'AND', 'OR', 'UNION', 'UNION ALL', 'EXCEPT', 'INTERSECT',
        'LIMIT', 'OFFSET', 'CASE', 'WHEN', 'THEN', 'ELSE', 'END',
        'CREATE TABLE', 'DROP TABLE', 'ALTER TABLE',
        'PRIMARY KEY', 'FOREIGN KEY', 'REFERENCES',
        'WITH', 'RECURSIVE',
      ].includes(k.toUpperCase())

      if (needsBreakBefore && current.trim()) {
        lines.push(current.trim())
        current = ''
      }
      current += (current.trim() ? ' ' : '') + k
      i += k.length
      continue
    }

    current += ch
    i++
  }

  if (current.trim()) lines.push(current.trim())

  return lines
    .filter(Boolean)
    .map(line => {
      const leadingParens = (line.match(/^\(+/) || [''])[0].length
      const trailingParens = (line.match(/\)+$/) || [''])[0].length
      const baseDepth = Math.max(0, leadingParens - trailingParens)
      let d = baseDepth
      return indentStr.repeat(d) + line
    })
    .join('\n')
}