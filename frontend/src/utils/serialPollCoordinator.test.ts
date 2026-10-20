import { describe, expect, it } from 'vitest'
import { SerialPollCoordinator } from './serialPollCoordinator'

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
}

describe('SerialPollCoordinator', () => {
  it('discards a paused read and immediately runs the resumed read', async () => {
    const coordinator = new SerialPollCoordinator()
    const stale = deferred<string>()
    const resumed = deferred<string>()
    const accepted: string[] = []

    const pausedPoll = coordinator.poll(() => stale.promise, (value) => { accepted.push(value) })
    coordinator.invalidate()
    const resumedPoll = coordinator.poll(() => resumed.promise, (value) => { accepted.push(value) })

    stale.resolve('stale chunk')
    await Promise.resolve()
    resumed.resolve('new chunk')
    await expect(pausedPoll).resolves.toBe(false)
    await expect(resumedPoll).resolves.toBe(true)
    expect(accepted).toEqual(['new chunk'])
  })

  it('does not apply an old file read after switching paths', async () => {
    const coordinator = new SerialPollCoordinator()
    const oldFile = deferred<string>()
    const accepted: string[] = []
    let path = 'old.log'

    const oldPoll = coordinator.poll(() => oldFile.promise, (value) => { accepted.push(path + ':' + value) })
    coordinator.invalidate()
    path = 'new.log'
    const newPoll = coordinator.poll(async () => 'new contents', (value) => { accepted.push(path + ':' + value) })

    oldFile.resolve('old contents')
    await expect(oldPoll).resolves.toBe(false)
    await expect(newPoll).resolves.toBe(true)
    expect(accepted).toEqual(['new.log:new contents'])
  })

  it('processes queued chunks in order without concurrent reads', async () => {
    const coordinator = new SerialPollCoordinator()
    const first = deferred<number>()
    const order: number[] = []

    const firstPoll = coordinator.poll(() => first.promise, (value) => { order.push(value) })
    const secondPoll = coordinator.poll(async () => 2, (value) => { order.push(value) })

    first.resolve(1)
    await expect(firstPoll).resolves.toBe(true)
    await expect(secondPoll).resolves.toBe(true)
    expect(order).toEqual([1, 2])
  })

  it('keeps only the newest queued read when timers tick repeatedly', async () => {
    const coordinator = new SerialPollCoordinator()
    const first = deferred<number>()
    const order: number[] = []

    const firstPoll = coordinator.poll(() => first.promise, (value) => { order.push(value) })
    const skippedPoll = coordinator.poll(async () => 2, (value) => { order.push(value) })
    const latestPoll = coordinator.poll(async () => 3, (value) => { order.push(value) })

    first.resolve(1)
    await expect(firstPoll).resolves.toBe(true)
    await expect(skippedPoll).resolves.toBe(false)
    await expect(latestPoll).resolves.toBe(true)
    expect(order).toEqual([1, 3])
  })
})
