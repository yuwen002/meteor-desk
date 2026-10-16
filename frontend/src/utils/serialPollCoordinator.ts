type PollRequest<T> = {
  generation: number
  run: () => Promise<T>
  accept: (result: T) => void | Promise<void>
  resolve: (accepted: boolean) => void
  reject: (error: unknown) => void
}

export class SerialPollCoordinator {
  private generation = 0
  private running = false
  private pending?: PollRequest<unknown>

  invalidate(): void {
    this.generation++
    this.pending?.resolve(false)
    this.pending = undefined
  }

  poll<T>(run: () => Promise<T>, accept: (result: T) => void | Promise<void>): Promise<boolean> {
    const request: PollRequest<T> = {
      generation: this.generation,
      run,
      accept,
      resolve: () => {},
      reject: () => {},
    }

    return new Promise<boolean>((resolve, reject) => {
      request.resolve = resolve
      request.reject = reject
      if (this.running) {
        this.pending?.resolve(false)
        this.pending = request as PollRequest<unknown>
        return
      }
      this.running = true
      void this.drain(request as PollRequest<unknown>)
    })
  }

  private async drain(first: PollRequest<unknown>): Promise<void> {
    let current: PollRequest<unknown> | undefined = first
    while (current) {
      try {
        const result = await current.run()
        if (current.generation !== this.generation) {
          current.resolve(false)
        } else {
          await current.accept(result)
          current.resolve(true)
        }
      } catch (error) {
        current.reject(error)
      }
      current = this.pending
      this.pending = undefined
    }
    this.running = false
  }
}
