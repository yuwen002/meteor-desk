export interface CustomRedactionRule {
  /** JavaScript regular-expression source; each match is replaced locally. */
  pattern: string
  replacement: string
}

export interface RedactionResult { text: string; replacements: number; invalidRules: number }

const sensitiveTokens = ['password', 'passwd', 'pwd', 'secret', 'token', 'apikey', 'api_key', 'authorization', 'cookie', 'session', 'privatekey', 'private_key', 'accesskey', 'access_key', 'credential']

export function isSensitiveField(name: string): boolean {
  const normalized = name.toLowerCase().replace(/[-_.\s]/g, '')
  return sensitiveTokens.some(token => normalized.includes(token.replace(/[_-]/g, '')))
}

/** Redacts common credential formats and optional user-defined regular expressions locally. */
export function redactSensitiveText(source: string, replacement = '[REDACTED]', customRules: CustomRedactionRule[] = []): RedactionResult {
  let replacements = 0
  let invalidRules = 0
  const replace = (matcher: RegExp, handler: (...args: string[]) => string) => {
    source = source.replace(matcher, (...args: string[]) => handler(...args))
  }
  replace(/("([^"\\]+)"\s*:\s*)"((?:\\.|[^"\\])*)"/g, (whole, prefix, key) => {
    if (!isSensitiveField(key)) return whole
    replacements++
    return prefix + '"' + replacement + '"'
  })
  replace(/(\bauthorization\b\s*[:=]\s*)(?:(?:bearer|basic)\s+)?([^\s,;]+)/gi, (whole, prefix) => {
    replacements++
    return prefix + replacement
  })
  replace(/(\b(?:x-api-key|api[-_]?key|password|passwd|pwd|secret|token|session(?:id)?|cookie)\b\s*[:=]\s*)([^\s,;"'}\]]+|"[^"]*"|'[^']*')/gi, (whole, prefix) => {
    replacements++
    return prefix + replacement
  })
  replace(/([?&]([^=&\s]+)=)([^&#\s]*)/g, (whole, prefix, key) => {
    if (!isSensitiveField(key)) return whole
    replacements++
    return prefix + encodeURIComponent(replacement)
  })
  replace(/(-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----)[\s\S]*?(-----END (?:RSA |EC |OPENSSH )?PRIVATE KEY-----)/g, (whole, begin, end) => {
    replacements++
    return begin + '\n' + replacement + '\n' + end
  })
  for (const rule of customRules) {
    if (!rule.pattern) continue
    try {
      const matcher = new RegExp(rule.pattern, 'g')
      source = source.replace(matcher, () => {
        replacements++
        return rule.replacement
      })
    } catch {
      invalidRules++
    }
  }
  return { text: source, replacements, invalidRules }
}
