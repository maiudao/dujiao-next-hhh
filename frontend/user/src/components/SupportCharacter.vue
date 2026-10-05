<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useAppStore } from '../stores/app'
import { useStorefrontMode } from '../composables/useStorefrontMode'
import { clampCharacterAnchor, differentIndex, readSupportState } from '../utils/supportCharacter'
import { X } from 'lucide-vue-next'

const appStore = useAppStore()
const { samplingMode } = useStorefrontMode()
const mode = computed(() => samplingMode.value ? 'sampling' : 'open')
const state = computed(() => readSupportState(appStore.config?.storefront_support?.[mode.value], mode.value))
const imageIndex = ref(0)
const messageIndex = ref(0)
const pressed = ref(false)
const bubbleOpen = ref(true)
const bubble = ref<HTMLElement | null>(null)
const failedImage = ref('')
const root = ref<HTMLElement | null>(null)
const position = ref<{ x: number; y: number } | null>(null)
const moved = ref(false)
const image = computed(() => state.value.images[imageIndex.value % state.value.images.length]!)
const fallbackImage = computed(() => samplingMode.value ? '/storefront/characters/gpt-busy.webp' : '/storefront/characters/gpt-normal.webp')
const message = computed(() => state.value.messages[messageIndex.value % state.value.messages.length])
let gesture: { id: number; x: number; y: number; left: number; top: number; dragged: boolean } | null = null
let animationTimer: ReturnType<typeof setTimeout> | undefined
let selectionSignature = ''

function pick(key: string, length: number) {
  let previous = -1
  try { previous = Number(sessionStorage.getItem(key) ?? -1) } catch { /* Storage is optional. */ }
  const index = differentIndex(length, previous)
  try { sessionStorage.setItem(key, String(index)) } catch { /* Keep working with storage disabled. */ }
  return index
}
function randomize() {
  const signature = JSON.stringify([mode.value, state.value])
  if (selectionSignature === signature) return
  selectionSignature = signature
  imageIndex.value = pick('shop-support-image-' + mode.value, state.value.images.length)
  messageIndex.value = pick('shop-support-message-' + mode.value, state.value.messages.length)
  failedImage.value = ''
}
function squish() {
  pressed.value = false
  void nextTick(() => {
    pressed.value = true
    clearTimeout(animationTimer)
    animationTimer = setTimeout(() => { pressed.value = false }, 260)
  })
}
function cycle() {
  messageIndex.value = (messageIndex.value + 1) % state.value.messages.length
  imageIndex.value = (imageIndex.value + 1) % state.value.images.length
  try {
    sessionStorage.setItem('shop-support-image-' + mode.value, String(imageIndex.value))
    sessionStorage.setItem('shop-support-message-' + mode.value, String(messageIndex.value))
  } catch { /* Keep working with storage disabled. */ }
  failedImage.value = ''
  squish()
}
function activateCharacter() {
  if (bubbleOpen.value) cycle()
  else { bubbleOpen.value = true; squish() }
}
function constrain(x: number, y: number) {
  const box = root.value?.getBoundingClientRect()
  if (!box) return
  const bubbleHeight = bubbleOpen.value ? (bubble.value?.offsetHeight || 0) + 14 : 0
  position.value = clampCharacterAnchor(x, y, box.width, box.height, bubbleHeight, window.innerWidth, window.innerHeight)
}
function startDrag(event: PointerEvent) {
  if (event.button !== 0) return
  const box = root.value?.getBoundingClientRect()
  if (!box) return
  gesture = { id: event.pointerId, x: event.clientX, y: event.clientY, left: box.left, top: box.top, dragged: false }
  ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
}
function drag(event: PointerEvent) {
  if (!gesture || event.pointerId !== gesture.id) return
  const dx = event.clientX - gesture.x
  const dy = event.clientY - gesture.y
  if (Math.hypot(dx, dy) > 5) gesture.dragged = true
  if (gesture.dragged) {
    moved.value = true
    constrain(gesture.left + dx, gesture.top + dy)
  }
}
function endDrag(event: PointerEvent) {
  if (!gesture || event.pointerId !== gesture.id) return
  // Pointer clicks are handled here so a drag never switches the message.
  if (!gesture.dragged && event.type !== 'pointercancel') activateCharacter()
  gesture = null
}
function resized() {
  if (!moved.value) position.value = null
  else if (position.value) constrain(position.value.x, position.value.y)
}
watch(state, randomize, { deep: true })
watch([message, bubbleOpen], async () => { await nextTick(); resized() })
onMounted(() => { randomize(); window.addEventListener('resize', resized) })
onUnmounted(() => { clearTimeout(animationTimer); window.removeEventListener('resize', resized) })
</script>

<template>
  <Teleport to="body">
  <aside ref="root" class="shop-character" :class="{ 'shop-character-pressed': pressed, 'shop-character-closed': samplingMode }"
    :style="position ? { left: position.x + 'px', top: position.y + 'px', right: 'auto', bottom: 'auto' } : undefined"
    aria-label="店铺客服提示">
    <div class="shop-character-reaction">
      <Transition name="support-bubble">
        <div v-if="bubbleOpen" ref="bubble" class="shop-character-bubble" id="shop-support-bubble">
          <button type="button" class="shop-character-copy" aria-label="切换客服提示" @click="cycle">
            <span class="shop-character-title"><i aria-hidden="true"></i>{{ samplingMode ? '打烊中' : '营业中' }}</span>
            <span class="shop-character-message" aria-live="polite">{{ message }}</span>
          </button>
          <button type="button" class="shop-character-close" aria-label="收起客服气泡" @click="bubbleOpen = false"><X :size="13" /></button>
        </div>
      </Transition>
    </div>
    <Transition name="support-tag">
      <button v-if="!bubbleOpen" type="button" class="shop-character-status" @click="activateCharacter" aria-label="展开客服气泡">
        <i aria-hidden="true"></i><span>{{ samplingMode ? '打烊中' : '营业中' }}</span>
      </button>
    </Transition>
    <button type="button" class="shop-character-handle" :aria-label="bubbleOpen ? '客服角色：点击切换提示，拖动调整位置' : '客服角色：点击展开气泡，拖动调整位置'"
      :aria-expanded="bubbleOpen" aria-controls="shop-support-bubble"
      @pointerdown="startDrag" @pointermove="drag" @pointerup="endDrag" @pointercancel="endDrag"
      @click="($event.detail === 0) && activateCharacter()">
      <img :src="failedImage === image ? fallbackImage : image" alt="店铺客服角色" draggable="false"
        @error="failedImage = image" />
    </button>
  </aside>
  </Teleport>
</template>

<style scoped>
.shop-character {
  --character-width: 125px; --character-height: 130px; --squish-distance: 15.6px;
  position: fixed; z-index: 45; width: 200px; pointer-events: none;
  height: var(--character-height);
  right: max(12px, calc((100% - 1132px) / 2 - 62.5px));
  bottom: max(34px, env(safe-area-inset-bottom));
  display: flex; justify-content: flex-end;
}
.shop-character-reaction { position: absolute; inset: auto 0 calc(100% + 10px); z-index: 3; }
.shop-character-bubble {
  position: relative; width: 100%; pointer-events: auto;
  text-align: left; background: var(--panel-bg, var(--ui-bg-elevated)); color: var(--ink, var(--ui-text-primary));
  border: 1.5px solid var(--line, var(--ui-border)); border-radius: 24px 26px 22px 28px;
  box-shadow: 0 8px 22px rgb(24 48 85 / 12%); cursor: pointer;
}
.shop-character-copy { display: block; width: 100%; max-height: max(80px, min(240px, calc(100dvh - var(--character-height) - 116px))); overflow-y: auto; padding: 12px 14px 14px; border: 0; background: transparent; color: inherit; text-align: left; cursor: pointer; border-radius: inherit; }
.shop-character-close { position: absolute; top: 7px; right: 7px; display: grid; place-items: center; width: 22px; height: 22px; border: 0; border-radius: 50%; color: #a52a3a; background: #ffe2e6; cursor: pointer; transition: background-color 140ms, color 140ms; }
.shop-character-close:hover { background: #cf3b50; color: #fff; }
:global(.dark) .shop-character-close { background: #512735; color: #ffb9c5; }
:global(.dark) .shop-character-close:hover { background: #ba344c; color: #fff; }
.shop-character-status { position: absolute; right: calc(var(--character-width) - 32px); bottom: 14px; z-index: 0; width: 100px; height: 32px; display: flex; align-items: center; gap: 5px; padding: 0 25px 0 10px; border: 1px solid #9ecbbf; border-radius: 16px 5px 5px 16px; color: #155d4d; background: #e4f3ed; box-shadow: 0 4px 12px rgb(24 48 85 / 10%); pointer-events: auto; cursor: pointer; font-size: 12px; font-weight: 800; white-space: nowrap; }
.shop-character-status i { width: 5px; height: 5px; flex-shrink: 0; border-radius: 50%; background: currentColor; }
.shop-character-closed .shop-character-status { color: #805110; background: #fff1d9; border-color: #dfc599; }
:global(.dark) .shop-character-status { color: #a5e1cf; background: #203d36; border-color: #3c6b5b; }
:global(.dark) .shop-character-closed .shop-character-status { color: #f4d39a; background: #443522; border-color: #746043; }
.shop-character-bubble::before, .shop-character-bubble::after {
  content: ''; position: absolute; right: 55px; width: 12px; height: 12px;
  border-radius: 0 0 4px 0; transform: rotate(45deg);
}
.shop-character-bubble::before { bottom: -8px; background: var(--line, var(--ui-border)); }
.shop-character-bubble::after { bottom: -6px; background: var(--panel-bg, var(--ui-bg-elevated)); }
.shop-character-title { display: flex; align-items: center; gap: 7px; padding-right: 18px; margin-bottom: 5px; color: var(--ui-accent); font-size: 14px; font-weight: 800; }
.shop-character-title i { width: 7px; height: 7px; border-radius: 50%; background: var(--green, #07835f); }
.shop-character-closed .shop-character-title i { background: var(--amber, #a56a00); }
.shop-character-message { display: block; font-size: 12px; line-height: 1.65; overflow-wrap: anywhere; }
.shop-character-handle { position: relative; z-index: 2; width: var(--character-width); height: var(--character-height); padding: 0; background: transparent; border: 0; pointer-events: auto; touch-action: none; cursor: grab; }
.shop-character-handle:active { cursor: grabbing; }
.shop-character-handle img { width: 100%; height: 100%; object-fit: contain; object-position: center bottom; user-select: none; filter: drop-shadow(0 6px 7px rgb(24 48 85 / 12%)); }
.shop-character-pressed img { animation: character-squish 260ms ease-out; transform-origin: center bottom; }
.shop-character-pressed .shop-character-reaction { animation: character-bubble-follow 260ms ease-out; }
.shop-character button:focus-visible { outline: 2px solid var(--ui-accent); outline-offset: 4px; }
@keyframes character-squish { 45% { transform: scale(1.07, .88); } }
@keyframes character-bubble-follow { 45% { transform: translateY(var(--squish-distance)); } }
.support-bubble-enter-active { transition: transform 240ms cubic-bezier(.16,1,.3,1), opacity 180ms; }
.support-bubble-leave-active { transition: transform 160ms ease-in, opacity 140ms; }
.support-bubble-enter-from, .support-bubble-leave-to { opacity: 0; transform: translateY(9px) scale(.9); }
.shop-character-bubble { transform-origin: 80% bottom; }
.support-tag-enter-active { transition: transform 250ms cubic-bezier(.16,1,.3,1) 70ms, opacity 130ms 70ms; }
.support-tag-leave-active { transition: transform 160ms ease-in, opacity 120ms; }
.support-tag-enter-from, .support-tag-leave-to { transform: translateX(62px); opacity: 0; }
@media (max-width: 900px) {
  .shop-character { --character-width: 88px; --character-height: 92px; --squish-distance: 11.04px; width: 160px; right: 16px; bottom: calc(24px + env(safe-area-inset-bottom)); }
  .shop-character-reaction { bottom: calc(100% + 8px); }
  .shop-character-bubble { border-radius: 20px 23px 19px 24px; }
  .shop-character-copy { padding: 10px 12px 12px; }
  .shop-character-title { font-size: 12px; margin-bottom: 3px; }
  .shop-character-message { font-size: 11px; line-height: 1.6; }
  .shop-character-bubble::before, .shop-character-bubble::after { right: 38px; }
}
@media (prefers-reduced-motion: reduce) {
  .shop-character-pressed img, .shop-character-pressed .shop-character-reaction { animation: none; }
  .support-bubble-enter-active, .support-bubble-leave-active, .support-tag-enter-active, .support-tag-leave-active { transition: opacity 100ms; }
  .support-bubble-enter-from, .support-bubble-leave-to, .support-tag-enter-from, .support-tag-leave-to { transform: none; }
}
</style>
