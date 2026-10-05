import assert from 'node:assert/strict'
import test from 'node:test'
import { createSupportAudio } from '../src/utils/supportAudio.ts'

const bundledAudio = 'data:audio/mpeg;base64,AQID'
const settle = async () => { for (let i = 0; i < 8; i++) await Promise.resolve() }

function audioFixture(delayedDecode = false) {
  const calls: { when: number; source: FakeSource }[] = []
  const decoders: (() => void)[] = []
  let resumed = 0
  let closed = false
  let decodes = 0
  class FakeSource {
    onended: (() => void) | null = null
    stopped = false
    buffer: unknown = null
    connect() {}
    disconnect() {}
    start(when: number) { calls.push({ when, source: this }) }
    stop() { this.stopped = true }
  }
  const context = {
    currentTime: 10,
    state: 'suspended',
    destination: {},
    resume: async () => { resumed++; context.state = 'running' },
    close: async () => { closed = true },
    createGain: () => ({ gain: { value: 1 }, connect() {} }),
    createBufferSource: () => new FakeSource(),
    decodeAudioData: async (data: ArrayBuffer) => {
      assert.deepEqual([...new Uint8Array(data)], [1, 2, 3])
      decodes++
      if (delayedDecode) await new Promise<void>(resolve => decoders.push(resolve))
      return { duration: .24 }
    },
  }
  const player = createSupportAudio(bundledAudio, bundledAudio, () => context as unknown as AudioContext)
  return { player, context, calls, decoders, stats: () => ({ resumed, closed, decodes }) }
}

test('first quick tap waits for decoding and Ya1 to finish, with only a short pause before Ya2', async () => {
  const f = audioFixture(true)
  f.player.play('down')
  f.player.play('up')
  assert.equal(f.stats().resumed, 1)
  assert.equal(f.calls.length, 0)
  f.decoders.forEach(resolve => resolve())
  await settle()
  assert.equal(f.calls.length, 1)
  f.context.currentTime = 10.24
  f.calls[0]!.source.onended?.()
  assert.equal(f.calls.length, 2)
  assert.ok(Math.abs(f.calls[1]!.when - 10.32) < 1e-6)
})

test('holding the character waits for release, then schedules Ya2 only 80ms later', async () => {
  const f = audioFixture()
  f.player.play('down')
  await settle()
  f.context.currentTime = 10.24
  f.calls[0]!.source.onended?.()
  assert.equal(f.calls.length, 1)
  f.context.currentTime = 11
  f.player.play('up')
  assert.equal(f.calls[1]!.when, 11.08)
})

test('cancel during decoding never plays late audio, and repeated taps cancel scheduled old sounds', async () => {
  const f = audioFixture(true)
  f.player.play('down')
  f.player.stop()
  f.decoders.forEach(resolve => resolve())
  await settle()
  assert.equal(f.calls.length, 0)
  f.player.play('down')
  f.player.play('up')
  await settle()
  f.context.currentTime = 10.24
  f.calls[0]!.source.onended?.()
  f.player.play('down')
  await settle()
  assert.equal(f.calls[1]!.source.stopped, true)
  assert.equal(f.stats().decodes, 2)
  f.player.dispose()
  assert.equal(f.stats().closed, true)
  assert.equal(f.calls[2]!.source.stopped, true)
})

test('a browser without Web Audio keeps the visual interaction usable', () => {
  const player = createSupportAudio(bundledAudio, bundledAudio, () => null)
  assert.doesNotThrow(() => { player.play('down'); player.play('up'); player.stop(); player.dispose() })
})
