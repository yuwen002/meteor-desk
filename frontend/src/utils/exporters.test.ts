import { describe, expect, it } from 'vitest'
import {
  buildProcessLogExportCSV,
  buildProcessLogExportJSON,
  buildSearchExportCSV,
  buildSearchExportJSON,
  processLogExportFileName,
  searchExportFileName,
  type SearchExportMeta,
} from './exporters'

const meta: SearchExportMeta = {
  filePath: 'C:/logs/app.log',
  query: 'boom',
  useRegex: false,
  encoding: 'utf-8',
  contextLines: 2,
  multiLineJSON: false,
  eventContext: true,
  resultLimitReached: false,
  canceled: false,
}

function match(overrides: Partial<LogMatch> = {}): LogMatch {
  return {
    lineNumber: 12,
    content: 'boom, "quoted"',
    contentClipped: false,
    contextBefore: [{ lineNumber: 11, content: 'before', contentClipped: false }],
    contextAfter: [{ lineNumber: 13, content: 'after', contentClipped: false }],
    ...overrides,
  }
}

describe('search result export', () => {
  it('writes one csv record per match and context line', () => {
    const csv = buildSearchExportCSV(meta, [match({ event: { startLine: 10, endLine: 14, content: 'event body\nsecond line', contentClipped: false, kind: 'event' } })])
    const lines = csv.split('\r\n').filter(Boolean)

    expect(lines[0].startsWith('\uFEFFmatchLine,type,lineNumber,endLine,query,content')).toBe(true)
    expect(lines).toHaveLength(5)
    expect(lines[1]).toBe('12,context-before,11,,boom,before')
    expect(lines[2]).toBe('12,match,12,,boom,"boom, ""quoted"""')
    expect(lines[3]).toBe('12,event,10,14,boom,"event body\nsecond line"')
    expect(lines[4]).toBe('12,context-after,13,,boom,after')
  })

  it('keeps the export metadata and full matches in json', () => {
    const json = JSON.parse(buildSearchExportJSON(meta, [match()], '2026-10-30T00:00:00.000Z'))

    expect(json).toMatchObject({
      exportedAt: '2026-10-30T00:00:00.000Z',
      filePath: 'C:/logs/app.log',
      query: 'boom',
      encoding: 'utf-8',
      total: 1,
    })
    expect(json.matches[0].contextAfter[0].content).toBe('after')
  })
})

describe('process log export', () => {
  const lines: ProcessOutput[] = [
    { sequence: 1, time: '10:00:00', stream: 'stdout', text: 'started' },
    { sequence: 2, time: '10:00:01', stream: 'stderr', text: 'failed, "retry"' },
  ]

  it('writes stream and text columns with escaped values', () => {
    const csv = buildProcessLogExportCSV(lines)
    const records = csv.split('\r\n').filter(Boolean)

    expect(records[0].startsWith('\uFEFFsequence,time,stream,text')).toBe(true)
    expect(records[1]).toBe('1,10:00:00,stdout,started')
    expect(records[2]).toBe('2,10:00:01,stderr,"failed, ""retry"""')
  })

  it('keeps the process identity in json', () => {
    const json = JSON.parse(buildProcessLogExportJSON('api', '后端 API', lines, '2026-10-30T00:00:00.000Z'))

    expect(json).toMatchObject({ processId: 'api', processName: '后端 API', total: 2 })
    expect(json.lines[1].stream).toBe('stderr')
  })
})

describe('export file names', () => {
  const now = new Date(2026, 9, 30, 8, 5, 9)

  it('uses the base log name and a local timestamp', () => {
    expect(searchExportFileName(meta, 'csv', now)).toBe('meteordesk-search-app.log-20261030-080509.csv')
    expect(searchExportFileName({ ...meta, filePath: '' }, 'json', now)).toBe('meteordesk-search-results-20261030-080509.json')
  })

  it('strips path separators and illegal characters from process names', () => {
    expect(processLogExportFileName('api/worker', 'csv', now)).toBe('meteordesk-process-api-worker-20261030-080509.csv')
    expect(processLogExportFileName('  ', 'json', now)).toBe('meteordesk-process-logs-20261030-080509.json')
  })
})
