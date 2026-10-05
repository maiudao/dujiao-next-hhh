export interface SupportAudio {
  play: (phase: 'down' | 'up') => void
  stop: () => void
  dispose: () => void
}

function browserContext(): AudioContext | null {
  const AudioContextClass = window.AudioContext
    || (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
  return AudioContextClass ? new AudioContextClass() : null
}

// Decode bundled data in memory. No media element, Blob URL or audio HTTP request.
export function createSupportAudio(
  downData: string,
  upData: string,
  createContext: () => AudioContext | null = browserContext,
): SupportAudio {
  let context: AudioContext | null = null
  let output: GainNode | null = null
  let buffers: Promise<[AudioBuffer, AudioBuffer]> | null = null
  const sources = new Set<AudioBufferSourceNode>()
  let generation = 0
  let disposed = false
  let released = false
  let firstFinished = false
  let startedAt = 0
  let releaseBuffer: AudioBuffer | null = null

  const stop = () => {
    generation++
    released = false
    firstFinished = false
    releaseBuffer = null
    for (const source of sources) {
      source.onended = null
      try { source.stop() } catch { /* Already ended. */ }
      source.disconnect()
    }
    sources.clear()
  }

  const start = (buffer: AudioBuffer, when: number, ended?: () => void) => {
    if (!context || !output) return
    const source = context.createBufferSource()
    source.buffer = buffer
    source.connect(output)
    source.onended = () => {
      sources.delete(source)
      source.disconnect()
      ended?.()
    }
    sources.add(source)
    source.start(when)
  }

  const release = () => {
    if (!context || !releaseBuffer || !released || !firstFinished) return
    released = false
    // A short pause after Ya1, while keeping quick taps' starts at least 320ms apart.
    const pause = Math.max(.08, .32 - (context.currentTime - startedAt))
    start(releaseBuffer, context.currentTime + pause)
  }

  const play = (phase: 'down' | 'up') => {
    if (disposed) return
    if (phase === 'up') {
      released = true
      release()
      return
    }
    stop()
    const currentGeneration = generation
    try {
      context ||= createContext()
      if (!context) return
      if (!output) {
        output = context.createGain()
        output.gain.value = .45
        output.connect(context.destination)
      }
      // Resume directly in the user gesture, before decoding the first tap's data.
      const ready = context.state === 'running' ? Promise.resolve() : context.resume()
      if (!buffers) {
        const decode = (data: string) => {
          const bytes = Uint8Array.from(atob(data.substring(data.indexOf(',') + 1)), c => c.charCodeAt(0))
          return context!.decodeAudioData(bytes.buffer)
        }
        buffers = Promise.all([decode(downData), decode(upData)])
      }
      void Promise.all([ready, buffers]).then(([, decoded]) => {
        if (disposed || generation !== currentGeneration || !context) return
        releaseBuffer = decoded[1]
        startedAt = context.currentTime
        start(decoded[0], startedAt, () => {
          firstFinished = true
          release()
        })
      }).catch(() => {
        if (generation === currentGeneration) {
          stop()
          buffers = null
        }
      })
    } catch {
      stop()
      buffers = null
    }
  }

  return {
    play,
    stop,
    dispose: () => {
      disposed = true
      stop()
      void context?.close().catch(() => {})
      context = null
      output = null
      buffers = null
    },
  }
}
