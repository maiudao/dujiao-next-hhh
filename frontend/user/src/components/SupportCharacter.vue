<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useAppStore } from '../stores/app'
import { useStorefrontMode } from '../composables/useStorefrontMode'
import { clampCharacterPosition, differentIndex, readSupportState } from '../utils/supportCharacter'

const appStore = useAppStore()
const { samplingMode } = useStorefrontMode()
const mode = computed(() => samplingMode.value ? 'sampling' : 'open')
const state = computed(() => readSupportState(appStore.config?.storefront_support?.[mode.value], mode.value))
const imageIndex = ref(0)
const messageIndex = ref(0)
const pressed = ref(false)
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
function cycle() {
  messageIndex.value = (messageIndex.value + 1) % state.value.messages.length
  imageIndex.value = (imageIndex.value + 1) % state.value.images.length
  try {
    sessionStorage.setItem('shop-support-image-' + mode.value, String(imageIndex.value))
    sessionStorage.setItem('shop-support-message-' + mode.value, String(messageIndex.value))
  } catch { /* Keep working with storage disabled. */ }
  failedImage.value = ''
  pressed.value = false
  void nextTick(() => {
    pressed.value = true
    clearTimeout(animationTimer)
    animationTimer = setTimeout(() => { pressed.value = false }, 240)
  })
}
function constrain(x: number, y: number) {
  const box = root.value?.getBoundingClientRect()
  if (!box) return
  position.value = clampCharacterPosition(x, y, box.width, box.height, window.innerWidth, window.innerHeight)
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
  if (!gesture.dragged && event.type !== 'pointercancel') cycle()
  gesture = null
}
function resized() {
  if (!moved.value) position.value = null
  else if (position.value) constrain(position.value.x, position.value.y)
}
watch(state, randomize, { deep: true })
onMounted(() => { randomize(); window.addEventListener('resize', resized) })
onUnmounted(() => { clearTimeout(animationTimer); window.removeEventListener('resize', resized) })
</script>

<template>
  <Teleport to="body">
  <aside ref="root" class="shop-character" :class="{ 'shop-character-pressed': pressed, 'shop-character-closed': samplingMode }"
    :style="position ? { left: position.x + 'px', top: position.y + 'px', right: 'auto', bottom: 'auto' } : undefined"
    aria-label="店铺客服提示">
    <button type="button" class="shop-character-bubble" @click="cycle">
      <span class="shop-character-title"><i aria-hidden="true"></i>{{ samplingMode ? '打样了' : '营业中' }}</span>
      <span class="shop-character-message" aria-live="polite">{{ message }}</span>
    </button>
    <button type="button" class="shop-character-handle" aria-label="客服角色：点击切换提示，拖动调整位置"
      @pointerdown="startDrag" @pointermove="drag" @pointerup="endDrag" @pointercancel="endDrag"
      @click="($event.detail === 0) && cycle()">
      <img :src="failedImage === image ? fallbackImage : image" alt="店铺客服角色" draggable="false"
        @error="failedImage = image" />
    </button>
  </aside>
  </Teleport>
</template>

<style scoped>
.shop-character {
  position: fixed; z-index: 45; width: 200px; pointer-events: none;
  right: max(12px, calc((100% - 1132px) / 2 - 62.5px));
  bottom: max(34px, env(safe-area-inset-bottom));
  display: flex; flex-direction: column; align-items: center; gap: 10px;
}
.shop-character-bubble {
  position: relative; width: 100%; padding: 12px 14px 14px; pointer-events: auto;
  text-align: left; background: var(--panel-bg, var(--ui-bg-elevated)); color: var(--ink, var(--ui-text-primary));
  border: 1.5px solid var(--line, var(--ui-border)); border-radius: 24px 26px 22px 28px;
  box-shadow: 0 8px 22px rgb(24 48 85 / 12%); cursor: pointer;
}
.shop-character-bubble::before, .shop-character-bubble::after {
  content: ''; position: absolute; right: 55px; width: 12px; height: 12px;
  border-radius: 0 0 4px 0; transform: rotate(45deg);
}
.shop-character-bubble::before { bottom: -8px; background: var(--line, var(--ui-border)); }
.shop-character-bubble::after { bottom: -6px; background: var(--panel-bg, var(--ui-bg-elevated)); }
.shop-character-title { display: flex; align-items: center; gap: 7px; margin-bottom: 5px; color: var(--ui-accent); font-size: 14px; font-weight: 800; }
.shop-character-title i { width: 7px; height: 7px; border-radius: 50%; background: var(--green, #07835f); }
.shop-character-closed .shop-character-title i { background: var(--amber, #a56a00); }
.shop-character-message { display: block; font-size: 12px; line-height: 1.65; overflow-wrap: anywhere; }
.shop-character-handle { align-self: flex-end; width: 125px; height: 130px; padding: 0; background: transparent; border: 0; pointer-events: auto; touch-action: none; cursor: grab; }
.shop-character-handle:active { cursor: grabbing; }
.shop-character-handle img { width: 100%; height: 100%; object-fit: contain; object-position: center bottom; user-select: none; filter: drop-shadow(0 6px 7px rgb(24 48 85 / 12%)); }
.shop-character-pressed img { animation: character-squish 240ms ease-out; transform-origin: center bottom; }
.shop-character button:focus-visible { outline: 2px solid var(--ui-accent); outline-offset: 4px; }
@keyframes character-squish { 45% { transform: scale(1.07, .88); } }
@media (max-width: 900px) {
  .shop-character { width: 160px; right: 16px; bottom: calc(24px + env(safe-area-inset-bottom)); gap: 8px; }
  .shop-character-bubble { padding: 10px 12px 12px; border-radius: 20px 23px 19px 24px; }
  .shop-character-title { font-size: 12px; margin-bottom: 3px; }
  .shop-character-message { font-size: 11px; line-height: 1.6; }
  .shop-character-handle { width: 88px; height: 92px; }
  .shop-character-bubble::before, .shop-character-bubble::after { right: 38px; }
}
@media (prefers-reduced-motion: reduce) { .shop-character-pressed img { animation: none; } }
</style>
