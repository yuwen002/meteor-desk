import { describe, expect, it } from 'vitest'
import { createSerialTaskQueue } from './serialTaskQueue'

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
}

describe('createSerialTaskQueue', () => {
  it('executes writes one at a time in invocation order', async () => {
    const queue = createSerialTaskQueue()
    const gate = deferred<void>()
    const events: string[] = []
    const first = queue(async () => {
      events.push('first:start')
      await gate.promise
      events.push('first:end')
      return 1
    })
    const second = queue(async () => {
      events.push('second:start')
      return 2
    })

    await Promise.resolve()
    expect(events).toEqual(['first:start'])
    gate.resolve()
    await expect(Promise.all([first, second])).resolves.toEqual([1, 2])
    expect(events).toEqual(['first:start', 'first:end', 'second:start'])
  })

  it('continues after a failed task', async () => {
    const queue = createSerialTaskQueue()
    const events: string[] = []
    const first = queue(async () => { events.push('failed'); throw new Error('disk full') })
    const second = queue(async () => { events.push('recovered'); return 'ok' })

    await expect(first).rejects.toThrow('disk full')
    await expect(second).resolves.toBe('ok')
    expect(events).toEqual(['failed', 'recovered'])
  })
})
