/** 解析 HTTP 调试器中的请求头输入；空输入视为不带自定义请求头。 */
export function parseRequestHeaders(raw: string): Record<string, string> {
  let parsed: unknown
  try {
    parsed = JSON.parse(raw.trim() || '{}')
  } catch (error) {
    // 保留原始解析异常作为 cause，便于控制台定位具体语法错误位置；界面只展示友好描述。
    throw new Error('请求头 JSON 无效：' + (error instanceof Error ? error.message : String(error)), { cause: error })
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('请求头必须是字符串键值对组成的 JSON 对象')
  const entries = Object.entries(parsed as Record<string, unknown>)
  if (entries.some(([, value]) => typeof value !== 'string')) throw new Error('请求头必须是字符串键值对组成的 JSON 对象')
  return Object.fromEntries(entries) as Record<string, string>
}
