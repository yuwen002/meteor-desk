export interface ParsedProcessForm {
  args: string[]
  env: Record<string, string>
}

/** Parse and validate JSON fields before passing a local process definition to Wails. */
export function parseProcessForm(argsText: string, envText: string): ParsedProcessForm {
  let parsedArgs: unknown
  let parsedEnv: unknown
  try {
    parsedArgs = JSON.parse(argsText)
    parsedEnv = JSON.parse(envText)
  } catch {
    throw new Error('参数或环境变量 JSON 格式无效，请检查括号、引号和逗号。')
  }

  if (!Array.isArray(parsedArgs) || !parsedArgs.every((item): item is string => typeof item === 'string')) {
    throw new Error('参数必须是 JSON 字符串数组，例如 ["run","dev"]')
  }
  if (parsedEnv === null || Array.isArray(parsedEnv) || typeof parsedEnv !== 'object'
    || !Object.values(parsedEnv as Record<string, unknown>).every(value => typeof value === 'string')) {
    throw new Error('环境变量必须是 JSON 字符串对象')
  }

  return { args: parsedArgs, env: parsedEnv as Record<string, string> }
}

/** Parse the comma separated port list used for local port diagnostics. */
export function parsePortList(text: string): number[] {
  const ports: number[] = []
  for (const part of text.split(/[,，\s]+/)) {
    const value = part.trim()
    if (!value) continue
    if (!/^\d{1,5}$/.test(value) || Number(value) < 1 || Number(value) > 65535) {
      throw new Error('监听端口需为 1 到 65535 之间的整数：' + value)
    }
    const port = Number(value)
    if (!ports.includes(port)) ports.push(port)
  }
  return ports.sort((left, right) => left - right)
}

export function isSensitiveEnvKey(key: string): boolean {
  const normalized = key.toLowerCase().replaceAll('_', '').replaceAll('-', '').replaceAll('.', '')
  return [
    'password', 'passwd', 'pwd', 'secret', 'token', 'apikey', 'accesskey',
    'privatekey', 'credential', 'authorization', 'cookie', 'signingkey',
    'clientsecret', 'dsn', 'databaseurl', 'dburl', 'redisurl', 'mongouri',
    'postgresurl', 'mysqlurl', 'jdbcurl', 'connectionstring', 'connectionuri',
    'webhook',
  ].some(part => normalized.includes(part))
}

/** Keep secret values off screen unless the user explicitly toggles the editor. */
export function getMaskedEnvironmentPreview(raw: string): string {
  try {
    const value: unknown = JSON.parse(raw)
    if (value === null || Array.isArray(value) || typeof value !== 'object'
      || !Object.values(value as Record<string, unknown>).every(item => typeof item === 'string')) {
      return '环境变量 JSON 无效，请进入编辑模式修正。'
    }
    const masked = Object.fromEntries(Object.entries(value as Record<string, string>).map(([key, item]) => [
      key, isSensitiveEnvKey(key) && item !== '' ? '••••••••' : item,
    ]))
    return JSON.stringify(masked, null, 2)
  } catch {
    return '环境变量 JSON 无效，请进入编辑模式修正。'
  }
}
