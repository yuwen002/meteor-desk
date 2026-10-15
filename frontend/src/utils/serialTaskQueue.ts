/**
 * Run asynchronous persistence tasks in the order they were requested.
 * A failed task does not poison the queue; later tasks can still run.
 */
export function createSerialTaskQueue() {
  let tail: Promise<void> = Promise.resolve()

  return function enqueue<T>(task: () => Promise<T>): Promise<T> {
    const result = tail.then(task)
    tail = result.then(
      () => undefined,
      () => undefined,
    )
    return result
  }
}
