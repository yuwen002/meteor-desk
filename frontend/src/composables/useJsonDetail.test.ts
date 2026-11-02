import { describe, expect, it } from 'vitest'
import { ref } from 'vue'
import { useJsonDetail } from './useJsonDetail'

function create(match: LogMatch | null) {
  const selectedMatch = ref<LogMatch | null>(match)
  const jsonFilter = ref('')
  const detail = useJsonDetail({ selectedMatch, jsonFilter })
  return { selectedMatch, jsonFilter, ...detail }
}

function row(content: string, extra?: Partial<LogMatch>): LogMatch {
  return { lineNumber: 3, content, contentClipped: false, contextBefore: [], contextAfter: [], ...extra }
}

describe('log match detail', () => {
  it('prefers the aggregated event content over the single line', () => {
    const detail = create(row('{"level":"error"}', { event: { startLine: 3, endLine: 9, content: '{"level":"error","stack":"boom"}', contentClipped: false, kind: 'event' } }))
    expect(detail.detailContent.value).toBe('{"level":"error","stack":"boom"}')
    expect(detail.detailEventLabel.value).toBe('事件')
    expect(detail.detailHint.value).toContain('异常堆栈')
    expect(detail.jsonFields.value.map(item => item.path)).toEqual(['level', 'stack'])
  })

  it('labels a multi-line JSON document differently and keeps the plain-line hint', () => {
    const document = create(row('{', { event: { startLine: 1, endLine: 4, content: '{"ok":true}', contentClipped: false, kind: 'document' } }))
    expect(document.detailEventLabel.value).toBe('文档')
    expect(document.detailHint.value).toContain('完整 JSON 文档')

    const plain = create(row('plain text line'))
    expect(plain.detailHint.value).toBe('查看前后文；JSON 行可格式化并按字段筛选。')
    expect(plain.parsedJSON.value).toBeNull()
    expect(plain.prettyJSON.value).toBe('')
  })

  it('does not parse content that the backend already clipped', () => {
    const detail = create(row('{"a":1}', { contentClipped: true }))
    expect(detail.detailContentClipped.value).toBe(true)
    expect(detail.parsedJSON.value).toBeNull()

    const event = create(row('{"a":1}', { event: { startLine: 1, endLine: 2, content: '{"a":', contentClipped: true, kind: 'event' } }))
    expect(event.detailContentClipped.value).toBe(true)
    expect(event.parsedJSON.value).toBeNull()
  })

  it('flattens nested objects and arrays into unambiguous field paths', () => {
    const detail = create(row(JSON.stringify({ service: 'api', tags: ['a', 'b'], meta: { emptyArray: [], emptyObject: {} }, 'bad key': 1 })))
    expect(detail.jsonFields.value.map(item => item.path)).toEqual(['service', 'tags[0]', 'tags[1]', 'meta.emptyArray', 'meta.emptyObject', '$["bad key"]'])
    expect(detail.jsonFields.value[3].value).toEqual([])
    expect(detail.jsonFields.value[4].value).toEqual({})
    expect(detail.prettyJSON.value).toContain('"tags": [')
  })

  it('filters fields by path or by serialized value', () => {
    const detail = create(row(JSON.stringify({ level: 'error', trace: { id: 'abc' } })))
    detail.jsonFilter.value = 'TRACE.'
    expect(detail.jsonFields.value.map(item => item.path)).toEqual(['trace.id'])

    detail.jsonFilter.value = 'ERROR'
    expect(detail.jsonFields.value.map(item => item.path)).toEqual(['level'])
  })

  it('stops at scalars, at eight levels of nesting and at three hundred fields', () => {
    const scalar = create(row('42'))
    expect(scalar.parsedJSON.value).toBeNull()

    const deep = (levels: number) => {
      let node: unknown = 'leaf'
      for (let index = levels; index >= 1; index--) node = { ['l' + index]: node }
      return create(row(JSON.stringify(node)))
    }
    expect(deep(8).jsonFields.value.map(item => item.path)).toEqual(['l1.l2.l3.l4.l5.l6.l7.l8'])
    expect(deep(10).jsonFields.value).toHaveLength(0)

    const wide = Object.fromEntries(Array.from({ length: 350 }, (_, index) => ['k' + index, index]))
    expect(create(row(JSON.stringify(wide))).jsonFields.value).toHaveLength(300)
  })

  it('falls back to the empty detail when nothing is selected', () => {
    const detail = create(null)
    expect(detail.detailContent.value).toBe('')
    expect(detail.detailContentClipped.value).toBe(false)
    expect(detail.jsonFields.value).toHaveLength(0)
  })
})
