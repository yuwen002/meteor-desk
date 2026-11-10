export type CompressionAlgorithm = 'gzip' | 'deflate' | 'brotli'
export const compressionAlgorithms: CompressionAlgorithm[] = ['gzip', 'deflate', 'brotli']

function encodeText(text: string): Uint8Array<ArrayBuffer> {
  return new TextEncoder().encode(text) as Uint8Array<ArrayBuffer>
}

export function compressionSupported(): Record<CompressionAlgorithm, boolean> {
  const ok = typeof CompressionStream !== 'undefined' && typeof DecompressionStream !== 'undefined'
  return { gzip: ok, deflate: ok, brotli: ok }
}

async function streamToBytes(stream: ReadableStream<Uint8Array>): Promise<Uint8Array> {
  const chunks: Uint8Array[] = []
  const reader = stream.getReader()
  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    if (value) chunks.push(value)
  }
  let total = 0
  for (const c of chunks) total += c.length
  const merged = new Uint8Array(total)
  let offset = 0
  for (const c of chunks) { merged.set(c, offset); offset += c.length }
  return merged
}

function pipeThrough<T>(source: ReadableStream<T>, transform: unknown): ReadableStream<T> {
  return (source as unknown as { pipeThrough: (t: unknown) => ReadableStream<T> }).pipeThrough(transform)
}

export async function compressText(text: string, algorithm: CompressionAlgorithm): Promise<Uint8Array> {
  if (!compressionSupported()[algorithm]) throw new Error(`当前环境不支持 ${algorithm} 压缩。`)
  const source = new ReadableStream<Uint8Array>({
    start(ctrl) { ctrl.enqueue(encodeText(text)); ctrl.close() },
  })
  const cs = new CompressionStream(algorithm as unknown as 'gzip')
  return streamToBytes(pipeThrough(source, cs))
}

export async function decompressToText(data: Uint8Array, algorithm: CompressionAlgorithm): Promise<string> {
  if (!compressionSupported()[algorithm]) throw new Error(`当前环境不支持 ${algorithm} 解压。`)
  const source = new ReadableStream<Uint8Array>({
    start(ctrl) { ctrl.enqueue(data); ctrl.close() },
  })
  const ds = new DecompressionStream(algorithm as unknown as 'gzip')
  const bytes = await streamToBytes(pipeThrough(source, ds))
  return new TextDecoder('utf-8', { fatal: true }).decode(bytes)
}

export function bytesToHex(bytes: Uint8Array): string {
  let hex = ''
  for (let o = 0; o < bytes.length; o += 0x4000) {
    const chunk = bytes.subarray(o, Math.min(o + 0x4000, bytes.length))
    hex += Array.from(chunk, b => b.toString(16).padStart(2, '0')).join('')
  }
  return hex
}

export function bytesToBase64(bytes: Uint8Array): string {
  let binary = ''
  for (let o = 0; o < bytes.length; o += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(o, Math.min(o + 0x8000, bytes.length)))
  }
  return btoa(binary)
}

export function base64ToBytes(b64: string): Uint8Array {
  const cleaned = b64.replace(/\s+/g, '').replace(/-/g, '+').replace(/_/g, '/')
  if (!/^[A-Za-z0-9+/]*={0,2}$/.test(cleaned) || cleaned.length % 4 === 1) throw new Error('Base64 字符串格式不正确。')
  const padded = cleaned + '='.repeat((4 - cleaned.length % 4) % 4)
  return Uint8Array.from(atob(padded), c => c.charCodeAt(0))
}