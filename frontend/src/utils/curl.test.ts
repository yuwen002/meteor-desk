import { describe, expect, it } from 'vitest'
import { buildCurlCommand, parseCurlCommand } from './curl'

function expectOk(input: string) {
  const result = parseCurlCommand(input)
  if (!result.ok) throw new Error('期望解析成功，实际失败：' + result.error)
  return result
}

describe('parseCurlCommand', () => {
  it('reads method, headers and body from a single-line command', () => {
    const result = expectOk(`curl -X POST 'https://api.example.com/items' -H 'Content-Type: application/json' -H "X-Trace: abc" --data-raw '{"name":"meteor"}'`)
    expect(result.request).toEqual({
      method: 'POST',
      url: 'https://api.example.com/items',
      headers: { 'Content-Type': 'application/json', 'X-Trace': 'abc' },
      body: '{"name":"meteor"}',
    })
    expect(result.warnings).toEqual([])
  })

  it('handles line continuations, quoted spaces and the compact flag forms', () => {
    const command = [
      'curl -XPOST https://api.example.com/search \\',
      "  -H 'Accept: text/plain' \\",
      "  --data-urlencode 'q=hello world' \\",
      '  --data-urlencode=a=1',
    ].join('\n')
    const result = expectOk(command)
    expect(result.request.method).toBe('POST')
    expect(result.request.url).toBe('https://api.example.com/search')
    expect(result.request.headers).toEqual({ Accept: 'text/plain' })
    expect(result.request.body).toBe('q=hello world&a=1')
  })

  it('defaults to POST when a body is present and GET otherwise', () => {
    expect(expectOk("curl https://example.com/health -d 'ping'").request).toEqual({
      method: 'POST',
      url: 'https://example.com/health',
      headers: {},
      body: 'ping',
    })
    expect(expectOk('curl https://example.com/health').request.method).toBe('GET')
  })

  it('maps the user agent, cookie and referer shortcuts to headers', () => {
    const result = expectOk("curl -A 'meteor/1.0' -b 'sid=1' -e 'https://ref.example.com' --url https://example.com/")
    expect(result.request.url).toBe('https://example.com/')
    expect(result.request.headers).toEqual({ 'User-Agent': 'meteor/1.0', Cookie: 'sid=1', Referer: 'https://ref.example.com' })
  })

  it('skips transport-only switches but reports unsupported options', () => {
    const result = expectOk('curl -k -L --compressed -s https://example.com/ --cert client.pem -H oops')
    expect(result.request.url).toBe('https://example.com/')
    expect(result.request.headers).toEqual({})
    // 未知选项的取值无法判断，会作为多余的位置参数提示出来。
    expect(result.warnings).toEqual(['忽略不支持的选项：--cert', '忽略多余的位置参数：client.pem', '忽略无效的请求头：oops'])
  })

  it('keeps the first positional argument as the URL and warns about extras', () => {
    const result = expectOk('curl https://example.com/a https://example.com/b')
    expect(result.request.url).toBe('https://example.com/a')
    expect(result.warnings).toEqual(['忽略多余的位置参数：https://example.com/b'])
  })

  it('reports clear errors for unusable input', () => {
    expect(parseCurlCommand('   ')).toEqual({ ok: false, error: '请先粘贴 cURL 命令。' })
    expect(parseCurlCommand('wget https://example.com')).toEqual({ ok: false, error: '命令需要以 curl 开头，例如 curl https://example.com。' })
    expect(parseCurlCommand("curl -H 'Accept: */*'")).toEqual({ ok: false, error: '未能从命令中识别请求 URL，请确认命令里包含完整地址。' })
    expect(parseCurlCommand("curl https://example.com -H 'Accept: */*")).toEqual({ ok: false, error: '命令中有未闭合的单引号。' })
    expect(parseCurlCommand('curl https://example.com -X')).toEqual({ ok: false, error: '选项 -X 缺少取值。' })
  })
})

describe('buildCurlCommand', () => {
  it('renders a multi-line command with headers and body', () => {
    const command = buildCurlCommand({
      method: 'post',
      url: 'https://api.example.com/items?page=1',
      headers: { Accept: 'application/json', 'X-Empty': '   ' },
      body: '{"name":"meteor"}',
    })
    expect(command).toBe([
      "curl -X POST 'https://api.example.com/items?page=1' \\",
      "  -H 'Accept: application/json' \\",
      "  --data-raw '{\"name\":\"meteor\"}'",
    ].join('\n'))
  })

  it('escapes single quotes so the command can be pasted back into a shell', () => {
    const command = buildCurlCommand({ method: 'POST', url: 'https://example.com/', headers: {}, body: "it's fine" })
    expect(command).toContain("--data-raw 'it'\\''s fine'")
    const reparsed = expectOk(command)
    expect(reparsed.request).toEqual({ method: 'POST', url: 'https://example.com/', headers: {}, body: "it's fine" })
  })

  it('round-trips a request through generate and parse', () => {
    const request = {
      method: 'PUT',
      url: 'https://api.example.com/users/42',
      headers: { Authorization: 'Bearer token', 'Content-Type': 'application/json' },
      body: '{"active":true}',
    }
    expect(expectOk(buildCurlCommand(request)).request).toEqual(request)
  })

  it('falls back to GET for an empty method and ignores blank headers', () => {
    expect(buildCurlCommand({ method: '', url: 'https://example.com/', headers: { '': 'x' }, body: '' })).toBe("curl -X GET 'https://example.com/'")
    expect(buildCurlCommand({ method: 'GET', url: '', headers: {}, body: '' })).toBe("curl -X GET ''")
  })
})
