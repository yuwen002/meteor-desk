import type { FieldFacet } from './fieldStats'

// 搜索结果与进程日志的导出转换：只负责生成文本，文件写入交给桌面桥接。
export type ExportFormat = 'csv' | 'json' | 'txt'

export interface SearchExportMeta {
  filePath: string
  query: string
  useRegex: boolean
  encoding: string
  contextLines: number
  multiLineJSON: boolean
  eventContext: boolean
  resultLimitReached: boolean
  canceled: boolean
}

const CSV_BOM = '\uFEFF'
// 导出文件名会直接交给操作系统写入，控制字符（\0-\x1f）必须洗掉，这里命中控制字符正是目的。
// eslint-disable-next-line no-control-regex -- 有意匹配控制字符以剔除它们
const ILLEGAL_FILE_CHARS = /[\\/:*?"<>|\u0000-\u001f]/g

function csvCell(value: string | number | boolean | undefined): string {
  if (value === undefined || value === null) return ''
  const text = String(value)
  return /[",\r\n]/.test(text) ? '"' + text.replace(/"/g, '""') + '"' : text
}

function csvRow(cells: (string | number | boolean | undefined)[]): string {
  return cells.map(csvCell).join(',')
}

// 每条匹配展开为多条记录，用 matchLine 关联上下文；表格工具里可直接按 type 筛选。
export function buildSearchExportCSV(meta: SearchExportMeta, matches: LogMatch[]): string {
  const rows = [csvRow(['matchLine', 'type', 'lineNumber', 'endLine', 'query', 'content'])]
  for (const match of matches) {
    for (const context of match.contextBefore) {
      rows.push(csvRow([match.lineNumber, 'context-before', context.lineNumber, '', meta.query, context.content]))
    }
    rows.push(csvRow([match.lineNumber, 'match', match.lineNumber, '', meta.query, match.content]))
    if (match.event) {
      rows.push(csvRow([match.lineNumber, match.event.kind ?? 'event', match.event.startLine, match.event.endLine, meta.query, match.event.content]))
    }
    for (const context of match.contextAfter) {
      rows.push(csvRow([match.lineNumber, 'context-after', context.lineNumber, '', meta.query, context.content]))
    }
  }
  return CSV_BOM + rows.join('\r\n') + '\r\n'
}

export function buildSearchExportJSON(meta: SearchExportMeta, matches: LogMatch[], exportedAt = new Date().toISOString()): string {
  return JSON.stringify({ exportedAt, ...meta, total: matches.length, matches }, null, 2)
}

export function buildProcessLogExportCSV(lines: ProcessOutput[]): string {
  const rows = [csvRow(['sequence', 'time', 'stream', 'text'])]
  for (const line of lines) rows.push(csvRow([line.sequence, line.time, line.stream, line.text]))
  return CSV_BOM + rows.join('\r\n') + '\r\n'
}

export function buildProcessLogExportJSON(processId: string, processName: string, lines: ProcessOutput[], exportedAt = new Date().toISOString()): string {
  return JSON.stringify({ exportedAt, processId, processName, total: lines.length, lines }, null, 2)
}

export function buildProcessLogExportTXT(lines: ProcessOutput[]): string {
  const streamLabel = (s: ProcessOutput['stream']) => s === 'stdout' ? 'OUT' : s === 'stderr' ? 'ERR' : 'SYS'
  return lines.map(l => `[${l.time}] [${streamLabel(l.stream)}] ${l.text}`).join('\r\n') + '\r\n'
}

function timestamp(date: Date): string {
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${date.getFullYear()}${pad(date.getMonth() + 1)}${pad(date.getDate())}-${pad(date.getHours())}${pad(date.getMinutes())}${pad(date.getSeconds())}`
}

function fileStem(value: string, fallback: string): string {
  const cleaned = value.replace(ILLEGAL_FILE_CHARS, '-').replace(/^[.\s]+|[.\s]+$/g, '').slice(0, 80)
  return cleaned || fallback
}

// 建议文件名只使用日志或服务自身的名称，避免把完整路径写进文件名。
export function searchExportFileName(meta: SearchExportMeta, format: ExportFormat, now = new Date()): string {
  const base = meta.filePath.split(/[\\/]/).filter(Boolean).pop() ?? ''
  return `meteordesk-search-${fileStem(base, 'results')}-${timestamp(now)}.${format}`
}

export function processLogExportFileName(processName: string, format: ExportFormat, now = new Date()): string {
  return `meteordesk-process-${fileStem(processName, 'logs')}-${timestamp(now)}.${format}`
}

// 字段取值分布导出为 CSV：先列 Top N 主取值，再追加「其他」与「缺失」汇总行，末尾附总计。
export function buildFacetExportCSV(facet: FieldFacet): string {
  const rows = [csvRow(['field', 'value', 'count', 'percentage'])]
  const total = Math.max(1, facet.total)
  const percent = (count: number) => (Math.round(count * 10000 / total) / 100).toFixed(2) + '%'
  for (const item of facet.values) {
    rows.push(csvRow([facet.path, item.value, item.count, percent(item.count)]))
  }
  if (facet.otherCount > 0) {
    rows.push(csvRow([facet.path, '(其他)', facet.otherCount, percent(facet.otherCount)]))
  }
  if (facet.missingCount > 0) {
    rows.push(csvRow([facet.path, '(缺失字段)', facet.missingCount, percent(facet.missingCount)]))
  }
  rows.push(csvRow([facet.path, '(总计)', facet.total, '100.00%']))
  return CSV_BOM + rows.join('\r\n') + '\r\n'
}

// 建议文件名用字段路径本身，超长时截断到安全长度。
export function facetExportFileName(fieldPath: string, format: ExportFormat, now = new Date()): string {
  const base = fileStem(fieldPath, 'facet').slice(0, 60)
  return `meteordesk-facet-${base}-${timestamp(now)}.${format}`
}