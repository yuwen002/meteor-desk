// appendJSONPath creates an unambiguous field path for arbitrary JSON object keys.
export function appendJSONPath(parent: string, key: string): string {
  if (/^[A-Za-z_$][A-Za-z0-9_$]*$/.test(key)) {
    return parent === '$' ? key : parent + '.' + key
  }
  const encodedKey = JSON.stringify(key)
  return parent === '$' ? '$[' + encodedKey + ']' : parent + '[' + encodedKey + ']'
}
