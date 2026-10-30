// cURL 与 HTTP 调试表单之间的互转：解析命令、生成可复制的命令文本。

export interface CurlRequest {
  method: string
  url: string
  headers: Record<string, string>
  body: string
}

export type CurlParseResult =
  | { ok: true; request: CurlRequest; warnings: string[] }
  | { ok: false; error: string }

// 需要取值的选项：短选项、长选项都映射到同一个字段。
const VALUE_FLAGS: Record<string, 'method' | 'header' | 'body' | 'url' | 'user-agent' | 'cookie' | 'referer'> = {
  '-X': 'method',
  '--request': 'method',
  '-H': 'header',
  '--header': 'header',
  '-d': 'body',
  '--data': 'body',
  '--data-raw': 'body',
  '--data-binary': 'body',
  '--data-ascii': 'body',
  '--data-urlencode': 'body',
  '--url': 'url',
  '-A': 'user-agent',
  '--user-agent': 'user-agent',
  '-b': 'cookie',
  '--cookie': 'cookie',
  '-e': 'referer',
  '--referer': 'referer',
}

// 省略取值的开关：只影响 cURL 的传输方式，导入时直接忽略。
const BOOLEAN_FLAGS = new Set([
  '--compressed', '-k', '--insecure', '-L', '--location', '--location-trusted',
  '-s', '--silent', '-v', '--verbose', '-i', '--include', '-f', '--fail',
  '--no-progress-meter', '-g', '--globoff', '-#', '--progress-bar', '-N', '--no-buffer',
])

type Tokenize = { tokens: string[] } | { error: string }

// tokenize 处理单/双引号、反斜杠转义与 \ 或 ^ 结尾的续行，足够覆盖常见 cURL 复制结果。
function tokenize(input: string): Tokenize {
  const normalized = input.replace(/\\\r?\n/g, ' ').replace(/\^\r?\n/g, ' ')
  const tokens: string[] = []
  let current = ''
  let started = false
  let quote: '' | "'" | '"' = ''
  for (let index = 0; index < normalized.length; index++) {
    const char = normalized[index]
    if (quote === "'") {
      if (char === "'") quote = ''
      else current += char
      continue
    }
    if (quote === '"') {
      if (char === '\\' && index + 1 < normalized.length) {
        const next = normalized[index + 1]
        if (next === '"' || next === '\\' || next === '$' || next === '`') {
          current += next
          index++
        } else {
          current += char
        }
        continue
      }
      if (char === '"') quote = ''
      else current += char
      continue
    }
    if (char === "'" || char === '"') {
      quote = char
      started = true
      continue
    }
    if (/\s/.test(char)) {
      if (started) tokens.push(current)
      current = ''
      started = false
      continue
    }
    if (char === '\\' && index + 1 < normalized.length) {
      current += normalized[index + 1]
      index++
      started = true
      continue
    }
    current += char
    started = true
  }
  if (quote) return { error: quote === "'" ? '命令中有未闭合的单引号。' : '命令中有未闭合的双引号。' }
  if (started) tokens.push(current)
  return { tokens }
}

// splitFlag 把 `--data-raw={"a":1}`、`-XPOST` 这类紧凑写法拆成选项与取值。
function splitFlag(token: string): { flag: string; inline?: string } {
  if (token.startsWith('--')) {
    const separator = token.indexOf('=')
    if (separator > 0) return { flag: token.slice(0, separator), inline: token.slice(separator + 1) }
    return { flag: token }
  }
  if (token.length > 2) return { flag: token.slice(0, 2), inline: token.slice(2) }
  return { flag: token }
}

// parseCurlCommand 把粘贴的 cURL 命令转成调试表单可用的请求；不支持的选项只给出提示。
export function parseCurlCommand(input: string): CurlParseResult {
  const trimmed = input.trim()
  if (!trimmed) return { ok: false, error: '请先粘贴 cURL 命令。' }
  const tokenized = tokenize(trimmed)
  if ('error' in tokenized) return { ok: false, error: tokenized.error }
  const tokens = tokenized.tokens
  if (!tokens.length) return { ok: false, error: '请先粘贴 cURL 命令。' }
  if (tokens[0].toLowerCase() !== 'curl') return { ok: false, error: '命令需要以 curl 开头，例如 curl https://example.com。' }

  const headers: Record<string, string> = {}
  const bodyParts: string[] = []
  const warnings: string[] = []
  let method = ''
  let url = ''
  for (let index = 1; index < tokens.length; index++) {
    const token = tokens[index]
    if (token === '--') continue
    if (!token.startsWith('-') || token === '-') {
      if (!url) url = token
      else warnings.push('忽略多余的位置参数：' + token)
      continue
    }
    const { flag, inline } = splitFlag(token)
    const field = VALUE_FLAGS[flag]
    let value = inline
    if (!field) {
      if (!inline && BOOLEAN_FLAGS.has(flag)) continue
      warnings.push('忽略不支持的选项：' + flag)
      continue
    }
    if (value === undefined) {
      value = tokens[index + 1]
      if (value === undefined) return { ok: false, error: '选项 ' + flag + ' 缺少取值。' }
      index++
    }
    if (field === 'method') {
      method = value.toUpperCase()
    } else if (field === 'url') {
      url = value
    } else if (field === 'body') {
      bodyParts.push(value)
    } else if (field === 'header') {
      const separator = value.indexOf(':')
      if (separator <= 0) {
        warnings.push('忽略无效的请求头：' + value)
        continue
      }
      headers[value.slice(0, separator).trim()] = value.slice(separator + 1).trim()
    } else {
      const name = field === 'user-agent' ? 'User-Agent' : field === 'cookie' ? 'Cookie' : 'Referer'
      headers[name] = value
    }
  }
  if (!url) return { ok: false, error: '未能从命令中识别请求 URL，请确认命令里包含完整地址。' }
  // 只带请求体、没写 -X 的 cURL 默认用 POST，与 curl 自身行为一致。
  if (!method) method = bodyParts.length ? 'POST' : 'GET'
  return { ok: true, request: { method, url, headers, body: bodyParts.join('&') }, warnings }
}

// shellQuote 用单引号包裹取值，内部单引号按 '\'' 转义，可直接粘回 shell。
function shellQuote(value: string): string {
  if (!value) return "''"
  return "'" + value.replace(/'/g, "'\\''") + "'"
}

// buildCurlCommand 把当前请求生成多行 cURL 命令；空请求头会被跳过。
export function buildCurlCommand(request: CurlRequest): string {
  const lines = ['curl -X ' + (request.method || 'GET').toUpperCase() + ' ' + shellQuote(request.url)]
  for (const [name, value] of Object.entries(request.headers)) {
    if (!name.trim() || !value.trim()) continue
    lines.push('  -H ' + shellQuote(name.trim() + ': ' + value.trim()))
  }
  if (request.body) lines.push('  --data-raw ' + shellQuote(request.body))
  return lines.join(' \\\n')
}
