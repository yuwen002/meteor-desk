import type { TailLevelFilter } from './logLevel'

export type TailSessionState = {
  filePath: string
  running: boolean
  paused: boolean
  text: string
  bytes: number
  offset: number
  levelFilter: TailLevelFilter
  error: string
  generation: number
}

export function createTailSession(filePath = ''): TailSessionState {
  return {
    filePath,
    running: false,
    paused: false,
    text: '',
    bytes: 0,
    offset: -1,
    levelFilter: 'ALL',
    error: '',
    generation: 0,
  }
}

export function startTailSession(session: TailSessionState, filePath: string): TailSessionState {
  return {
    ...session,
    filePath,
    running: true,
    paused: false,
    text: '',
    bytes: 0,
    offset: -1,
    error: '',
    generation: session.generation + 1,
  }
}

export function stopTailSession(session: TailSessionState): TailSessionState {
  return { ...session, running: false, paused: false, generation: session.generation + 1 }
}

export function pauseTailSession(session: TailSessionState): TailSessionState {
  if (!session.running) return session
  return { ...session, paused: !session.paused, generation: session.generation + 1 }
}

export function clearTailSession(session: TailSessionState): TailSessionState {
  return { ...session, text: '', bytes: 0 }
}

export function appendTailChunk(
  session: TailSessionState,
  chunk: { content: string; offset: number; reset: boolean },
  maxBytes = 1024 * 1024,
): TailSessionState {
  const base = chunk.reset ? { ...session, text: '', bytes: 0 } : session
  const content = base.text + chunk.content
  const bounded = content.slice(-maxBytes)
  return {
    ...base,
    text: bounded,
    bytes: base.bytes + new TextEncoder().encode(chunk.content).length,
    offset: chunk.offset,
    error: '',
  }
}
