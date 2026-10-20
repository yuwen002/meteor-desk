import { describe, expect, it } from 'vitest'
import {
  appendTailChunk,
  clearTailSession,
  createTailSession,
  pauseTailSession,
  startTailSession,
  stopTailSession,
} from './tailSession'

describe('independent tail session state', () => {
  it('starts from EOF and resets only the selected session', () => {
    const first = startTailSession(createTailSession(), '/tmp/one.log')
    const second = startTailSession(createTailSession(), '/tmp/two.log')
    const updated = appendTailChunk(first, { content: 'new line\n', offset: 9, reset: false })

    expect(updated.offset).toBe(9)
    expect(updated.text).toBe('new line\n')
    expect(second.text).toBe('')
    expect(second.offset).toBe(-1)
  })

  it('clears displayed output without stopping the session or resetting its offset', () => {
    const running = appendTailChunk(
      startTailSession(createTailSession(), 'service.log'),
      { content: 'line\n', offset: 5, reset: false },
    )
    const cleared = clearTailSession(running)

    expect(cleared.text).toBe('')
    expect(cleared.bytes).toBe(0)
    expect(cleared.offset).toBe(5)
    expect(cleared.running).toBe(true)
  })

  it('resets output when the file is rotated or truncated', () => {
    const session = appendTailChunk(
      startTailSession(createTailSession(), 'service.log'),
      { content: 'old contents', offset: 12, reset: false },
    )
    const rotated = appendTailChunk(session, { content: 'fresh', offset: 5, reset: true })

    expect(rotated.text).toBe('fresh')
    expect(rotated.bytes).toBe(5)
    expect(rotated.offset).toBe(5)
  })

  it('toggles pause and stop without affecting another session', () => {
    const running = startTailSession(createTailSession(), 'one.log')
    const paused = pauseTailSession(running)
    const stopped = stopTailSession(paused)

    expect(paused.paused).toBe(true)
    expect(stopped.running).toBe(false)
    expect(stopped.paused).toBe(false)
    expect(stopped.generation).toBeGreaterThan(running.generation)
    expect(running.running).toBe(true)
  })
})
