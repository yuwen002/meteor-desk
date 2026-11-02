import { computed, type Ref } from 'vue'
import { appendJSONPath } from '../utils/jsonPath'

// 详情面板的字段展平上限：超过 300 项或 8 层就停止下钻，避免深嵌套 JSON 拖慢渲染。
const MAX_JSON_FIELDS = 300
const MAX_JSON_DEPTH = 8

/**
 * 选中行的详情状态：优先展示聚合事件内容，JSON 行格式化后展平成字段列表供过滤。
 * 只依赖 selectedMatch 与 jsonFilter，因此可以脱离模板单独验证。
 */
export function useJsonDetail(options: { selectedMatch: Ref<LogMatch | null>; jsonFilter: Ref<string> }) {
  const detailContent = computed(() => options.selectedMatch.value?.event?.content || options.selectedMatch.value?.content || '')
  const detailContentClipped = computed(() => {
    const event = options.selectedMatch.value?.event
    if (event?.content) return event.contentClipped
    return options.selectedMatch.value?.contentClipped ?? false
  })
  const detailEventLabel = computed(() => options.selectedMatch.value?.event?.kind === 'document' ? '文档' : '事件')
  const detailHint = computed(() => {
    const event = options.selectedMatch.value?.event
    if (!event) return '查看前后文；JSON 行可格式化并按字段筛选。'
    return event.kind === 'document' ? '完整 JSON 文档已保留，可格式化并筛选字段。' : '完整事件上下文已聚合；异常堆栈和多行 JSON 会保留在同一事件内。'
  })
  // 被截断的内容不尝试解析，防止把半截 JSON 当成结构展示。
  const parsedJSON = computed(() => {
    if (!options.selectedMatch.value || detailContentClipped.value) return null
    try {
      const value: unknown = JSON.parse(detailContent.value)
      return value !== null && typeof value === 'object' ? value : null
    } catch {
      return null
    }
  })
  const prettyJSON = computed(() => parsedJSON.value === null ? '' : JSON.stringify(parsedJSON.value, null, 2))
  const jsonFields = computed(() => {
    const out: JsonField[] = []
    const add = (value: unknown, path: string, depth: number) => {
      if (out.length >= MAX_JSON_FIELDS || depth > MAX_JSON_DEPTH) return
      if (Array.isArray(value)) {
        if (!value.length) out.push({ path, value: [] })
        value.forEach((item, index) => add(item, path + '[' + index + ']', depth + 1))
      } else if (value && typeof value === 'object') {
        const entries = Object.entries(value as Record<string, unknown>)
        if (!entries.length) out.push({ path, value: {} })
        for (const [key, item] of entries) add(item, appendJSONPath(path, key), depth + 1)
      } else out.push({ path, value })
    }
    if (parsedJSON.value !== null) add(parsedJSON.value, '$', 0)
    const keyword = options.jsonFilter.value.trim().toLowerCase()
    return keyword ? out.filter(item => item.path.toLowerCase().includes(keyword) || JSON.stringify(item.value).toLowerCase().includes(keyword)) : out
  })

  return { detailContent, detailContentClipped, detailEventLabel, detailHint, parsedJSON, prettyJSON, jsonFields }
}
