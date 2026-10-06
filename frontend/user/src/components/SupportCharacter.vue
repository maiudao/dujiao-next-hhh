<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useAppStore } from '../stores/app'
import { useStorefrontMode } from '../composables/useStorefrontMode'
import { clampCharacterAnchor, differentIndex, readSupportState, supportMessagePool } from '../utils/supportCharacter'
import { createSupportAudio } from '../utils/supportAudio'
import downSoundData from '../assets/support-sounds/Ya1.mp3?inline'
import upSoundData from '../assets/support-sounds/Ya2.mp3?inline'
import { X, MessageCircle } from 'lucide-vue-next'
import ContactDialog from './ContactDialog.vue'
import { usePaidOrderPage } from '../composables/usePaidOrderSupport'
import { readPaidOrderMessage } from '../utils/paidOrderSupport'

const appStore = useAppStore()
const { samplingMode } = useStorefrontMode()
const mode = computed(() => samplingMode.value ? 'sampling' : 'open')
const state = computed(() => readSupportState(appStore.config?.storefront_support?.[mode.value], mode.value))
const messages = computed(() => supportMessagePool(state.value))
const imageIndex = ref(0)
const messageIndex = ref(0)
const pressed = ref(false)
const bubbleOpen = ref(false)
const contactOpen = ref(false)
const { key: paidOrderPage, phase: orderPhase } = usePaidOrderPage()
const bubble = ref<HTMLElement | null>(null)
const bubbleSize = ref({ width: 200, height: 100 })
const statusTitle = computed(() => samplingMode.value ? '打烊中' : '正在营业')
const bubbleTitle = computed(() => orderPhase.value === 'delivered' ? '交付成功' : orderPhase.value === 'paid' ? '订单已付款' : statusTitle.value)
// Pixel coordinates keep the rounded corner stable when the message height changes.
const bubblePath = computed(() => {
  const w = bubbleSize.value.width, h = bubbleSize.value.height
  // A regular 24px corner; the close button sits midway along its outer arc.
  return `M 25 1 H ${w - 25} A 24 24 0 0 1 ${w - 1} 25 V ${h - 25} Q ${w - 1} ${h - 1} ${w - 25} ${h - 1} H ${w - 43} Q ${w - 46} ${h - 1} ${w - 48} ${h + 1} L ${w - 60} ${h + 11} Q ${w - 63} ${h + 13} ${w - 65} ${h + 10} L ${w - 75} ${h + 1} Q ${w - 77} ${h - 1} ${w - 80} ${h - 1} H 25 Q 1 ${h - 1} 1 ${h - 25} V 25 Q 1 1 25 1 Z`
})
const failedImage = ref('')
const root = ref<HTMLElement | null>(null)
const position = ref<{ x: number; y: number } | null>(null)
const moved = ref(false)
const image = computed(() => state.value.images[imageIndex.value % state.value.images.length]!)
const fallbackImage = computed(() => samplingMode.value ? '/storefront/characters/gpt-busy.webp' : '/storefront/characters/gpt-normal.webp')
const currentMessage = computed(() => messages.value[messageIndex.value % messages.value.length]!)
const showContact = computed(() => Boolean(paidOrderPage.value) || currentMessage.value.showContact)
const message = computed(() => paidOrderPage.value
  ? readPaidOrderMessage(orderPhase.value === 'delivered'
    ? appStore.config?.storefront_support?.delivered_order_message
    : appStore.config?.storefront_support?.paid_order_message, orderPhase.value === 'delivered' ? 'delivered' : 'paid')
  : currentMessage.value.text)
let gesture: { id: number; x: number; y: number; left: number; top: number; dragged: boolean } | null = null
let animationTimer: ReturnType<typeof setTimeout> | undefined
let selectionSignature = ''
let bubbleObserver: ResizeObserver | undefined
const supportAudio = createSupportAudio(downSoundData, upSoundData)
const playSound = supportAudio.play
const stopSounds = supportAudio.stop

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
  messageIndex.value = pick('shop-support-message-' + mode.value, messages.value.length)
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
  if (!paidOrderPage.value) messageIndex.value = differentIndex(messages.value.length, messageIndex.value)
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
  playSound('down')
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
  if (event.type !== 'pointercancel') playSound('up')
  else stopSounds()
  // Pointer clicks are handled here so a drag never switches the message.
  if (!gesture.dragged && event.type !== 'pointercancel') activateCharacter()
  gesture = null
}
function resized() {
  if (!moved.value) position.value = null
  else if (position.value) constrain(position.value.x, position.value.y)
}
watch(state, randomize, { deep: true })
watch(paidOrderPage, (orderNo, previous) => {
  if (orderNo) bubbleOpen.value = true
  else if (previous) bubbleOpen.value = false
}, { immediate: true })
watch(bubble, element => {
  bubbleObserver?.disconnect()
  if (!element) return
  const measure = () => { bubbleSize.value = { width: element.offsetWidth, height: element.offsetHeight } }
  measure()
  bubbleObserver = new ResizeObserver(measure)
  bubbleObserver.observe(element)
})
watch([message, bubbleOpen], async () => { await nextTick(); resized() })
onMounted(() => { randomize(); window.addEventListener('resize', resized) })
onUnmounted(() => {
  clearTimeout(animationTimer); bubbleObserver?.disconnect(); window.removeEventListener('resize', resized)
  supportAudio.dispose()
})
</script>

<template>
  <Teleport to="body">
  <aside ref="root" class="shop-character" :class="{ 'shop-character-pressed': pressed, 'shop-character-closed': samplingMode }"
    :style="position ? { left: position.x + 'px', top: position.y + 'px', right: 'auto', bottom: 'auto' } : undefined"
    aria-label="店铺客服提示">
    <div class="shop-character-reaction" :class="{ 'shop-character-with-contact': showContact }">
      <Transition name="support-bubble">
        <div v-if="bubbleOpen" ref="bubble" class="shop-character-bubble" id="shop-support-bubble">
          <svg class="shop-character-outline" :viewBox="`0 0 ${bubbleSize.width} ${bubbleSize.height}`" preserveAspectRatio="none" aria-hidden="true"><path :d="bubblePath" /></svg>
          <button type="button" class="shop-character-copy" aria-label="切换客服提示" @click="cycle"
            @pointerdown="playSound('down')" @pointerup="playSound('up')" @pointercancel="stopSounds"
            @keydown="(['Enter', ' '].includes($event.key) && !$event.repeat) && playSound('down')"
            @keyup="['Enter', ' '].includes($event.key) && playSound('up')">
            <span class="shop-character-title"><i aria-hidden="true"></i>{{ bubbleTitle }}<span v-if="!paidOrderPage" class="status-dots" aria-hidden="true"><b>.</b><b>.</b><b>.</b></span></span>
            <span class="shop-character-message" aria-live="polite">{{ message }}</span>
          </button>
          <button v-if="showContact" type="button" class="shop-character-contact" @click="contactOpen = true"><MessageCircle :size="14" aria-hidden="true" /><span>获取店家的联系方式</span></button>
          <button type="button" class="shop-character-close" aria-label="收起客服气泡" @click="bubbleOpen = false"><X :size="10" /></button>
        </div>
      </Transition>
    </div>
    <Transition name="support-tag">
      <button v-if="!bubbleOpen" type="button" class="shop-character-status" @click="activateCharacter" aria-label="展开客服气泡">
        <svg class="status-ribbon" viewBox="0 0 114 34" preserveAspectRatio="none" aria-hidden="true"><path d="M18 1H97Q101 1 104 5L113 17L104 29Q101 33 97 33H18A16 16 0 0 1 18 1Z" /></svg>
        <i aria-hidden="true"></i><span>{{ statusTitle }}<span class="status-dots" aria-hidden="true"><b>.</b><b>.</b><b>.</b></span></span>
      </button>
    </Transition>
    <button type="button" class="shop-character-handle" :aria-label="bubbleOpen ? '客服角色：点击切换提示，拖动调整位置' : '客服角色：点击展开气泡，拖动调整位置'"
      :aria-expanded="bubbleOpen" aria-controls="shop-support-bubble"
      @pointerdown="startDrag" @pointermove="drag" @pointerup="endDrag" @pointercancel="endDrag"
      @keydown="(['Enter', ' '].includes($event.key) && !$event.repeat) && playSound('down')"
      @keyup="['Enter', ' '].includes($event.key) && playSound('up')"
      @click="($event.detail === 0) && activateCharacter()">
      <img :src="failedImage === image ? fallbackImage : image" alt="店铺客服角色" draggable="false"
        @error="failedImage = image" />
    </button>
  </aside>
  </Teleport>
  <ContactDialog v-model:open="contactOpen" />
</template>

<style scoped>
.shop-character {
  --character-width: 125px; --character-height: 130px; --squish-distance: 15.6px;
  position: fixed; z-index: 45; width: 234px; pointer-events: none;
  height: var(--character-height);
  right: max(12px, calc((100% - 1132px) / 2 - 62.5px));
  bottom: max(34px, env(safe-area-inset-bottom));
  display: flex; justify-content: flex-end;
}
.shop-character-reaction { position: absolute; right: 0; bottom: calc(100% + 10px); width: 200px; z-index: 3; }
.shop-character-with-contact { width: 220px; }
.shop-character-bubble {
  position: relative; display: flow-root; width: 100%; pointer-events: auto;
  text-align: left; color: #29334b; cursor: pointer;
  filter: drop-shadow(0 7px 11px rgb(24 48 85 / 14%));
}
.shop-character-outline { position: absolute; inset: 0; width: 100%; height: 100%; overflow: visible; pointer-events: none; }
.shop-character-outline path { fill: #fff; stroke: #d8deeb; stroke-width: 1.25; vector-effect: non-scaling-stroke; }
.shop-character-copy { position: relative; display: block; width: 100%; min-height: 86px; max-height: max(86px, min(240px, calc(100dvh - var(--character-height) - 116px))); overflow-y: auto; padding: 14px 16px 18px; border: 0; background: transparent; color: inherit; text-align: left; cursor: pointer; border-radius: 24px; }
/* The 28px hit area centers at (width - 8, 8), the corner arc's 45-degree midpoint. */
.shop-character-close { position: absolute; top: -6px; right: -6px; display: grid; place-items: center; width: 28px; height: 28px; border: 0; border-radius: 50%; color: #a52a3a; background: transparent; cursor: pointer; }
.shop-character-close::before { content: ''; position: absolute; width: 17px; height: 17px; box-sizing: border-box; border: 1.25px solid #cb8e98; border-radius: 50%; background: #ffe2e6; transition: background-color 140ms, border-color 140ms; }
.shop-character-close svg { position: relative; }
.shop-character-close:hover { color: #fff; }
.shop-character-close:hover::before { background: #cf3b50; border-color: #a52a3a; }
.shop-character-status { position: absolute; right: calc(var(--character-width) - 38px); bottom: 13px; z-index: 0; width: 114px; height: 34px; display: flex; align-items: center; gap: 6px; padding: 0 0 0 12px; border: 0; color: #205b47; background: transparent; filter: drop-shadow(0 4px 5px rgb(24 48 85 / 12%)); pointer-events: auto; cursor: pointer; font-size: 11px; font-weight: 800; white-space: nowrap; }
.status-ribbon { position: absolute; inset: 0; width: 100%; height: 100%; z-index: -1; overflow: visible; }
.status-ribbon path { fill: #f0faf3; stroke: #a4cebc; stroke-width: 1; }
.shop-character-status i { width: 5px; height: 5px; flex-shrink: 0; border-radius: 50%; background: currentColor; box-shadow: 0 0 0 3px rgb(40 130 90 / 9%); }
.shop-character-closed .shop-character-status { color: #815212; }
.shop-character-closed .status-ribbon path { fill: #fff4e1; stroke: #dfc599; }
.status-dots { display: inline-flex; width: 14px; margin-left: 1px; vertical-align: baseline; }
.status-dots b { font-weight: inherit; }
.status-dots b:nth-child(2) { animation: status-dot-two 1.8s steps(1, end) infinite; }
.status-dots b:nth-child(3) { animation: status-dot-three 1.8s steps(1, end) infinite; }
@keyframes status-dot-two { 0%, 100% { opacity: 0; } 33.33%, 99.99% { opacity: 1; } }
@keyframes status-dot-three { 0%, 100% { opacity: 0; } 66.66%, 99.99% { opacity: 1; } }
.shop-character-title { display: flex; align-items: center; gap: 6px; padding-right: 24px; margin-bottom: 7px; color: #215fc6; font-size: 13px; font-weight: 800; }
.shop-character-title .status-dots { margin-left: -5px; }
.shop-character-title i { width: 7px; height: 7px; border-radius: 50%; background: var(--green, #07835f); }
.shop-character-closed .shop-character-title i { background: var(--amber, #a56a00); }
.shop-character-message { display: block; font-size: 12px; font-weight: 400; line-height: 1.65; overflow-wrap: anywhere; }
.shop-character-contact { position: relative; display: flex; align-items: center; justify-content: center; gap: 6px; width: calc(100% - 28px); min-height: 36px; margin: 0 14px 18px; padding: 8px; border: 1px solid #245cb0; border-radius: 8px; background: #2864bd; color: #fff; font-size: 11px; font-weight: 600; line-height: 1.45; cursor: pointer; pointer-events: auto; transition: background-color 140ms, border-color 140ms; }
.shop-character-contact svg { flex: none; }
.shop-character-contact span { white-space: nowrap; }
.shop-character-contact:hover { background: #1e54a4; border-color: #1e54a4; }
.shop-character-contact:active { background: #194887; }
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
.support-tag-enter-active { transition: transform 320ms cubic-bezier(.16,1,.3,1) 70ms, opacity 130ms 70ms; }
.support-tag-leave-active { transition: transform 160ms ease-in, opacity 120ms; }
.support-tag-enter-from, .support-tag-leave-to { transform: translateX(62px); opacity: 0; }
@media (max-width: 900px) {
  .shop-character { --character-width: 88px; --character-height: 92px; --squish-distance: 11.04px; width: 188px; right: 16px; bottom: calc(24px + env(safe-area-inset-bottom)); }
  .shop-character-reaction { bottom: calc(100% + 8px); width: 160px; }
  .shop-character-with-contact { width: 180px; }
  .shop-character-contact { gap: 5px; width: calc(100% - 24px); margin-inline: 12px; font-size: 10.5px; }
  .shop-character-bubble { border-radius: 20px 23px 19px 24px; }
  .shop-character-copy { padding: 12px 13px 16px; }
  .shop-character-title { font-size: 12px; margin-bottom: 3px; }
  .shop-character-message { font-size: 11px; line-height: 1.6; }
  .shop-character-status { height: 32px; font-size: 10px; padding-left: 12px; gap: 6px; }
}
@media (prefers-reduced-motion: reduce) {
  .shop-character-pressed img, .shop-character-pressed .shop-character-reaction { animation: none; }
  .status-dots b { animation: none; opacity: 1; }
  .support-bubble-enter-active, .support-bubble-leave-active, .support-tag-enter-active, .support-tag-leave-active { transition: opacity 100ms; }
  .support-bubble-enter-from, .support-bubble-leave-to, .support-tag-enter-from, .support-tag-leave-to { transform: none; }
}
</style>
