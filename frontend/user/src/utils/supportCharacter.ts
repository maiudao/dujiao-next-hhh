import characterCatalog from '../../public/storefront/characters/catalog.json' with { type: 'json' }

export interface SupportState {
  images: string[]
  messages: string[]
  contact_messages: string[]
}

export interface SupportMessage { text: string; showContact: boolean }

export const supportDefaults: Record<'open' | 'sampling', SupportState> = {
  open: {
    images: characterCatalog.open.map(item => item.path),
    messages: [
      '购买前后有什么不清楚的，都可以和客服保持联系哦 (^_^)',
      '想了解商品或库存？下单前可以先和客服聊聊。',
      '付款后记得查看订单进度，有售后问题也可以联系店主。',
    ],
    contact_messages: ['想了解商品、购买流程或售后？可以添加店主的联系方式，先聊清楚再下单。'],
  },
  sampling: {
    images: characterCatalog.sampling.map(item => item.path),
    messages: [
      '明天再来吧，营业时间是每天9：00到12：00。',
      '店主正在休息，营业后就能继续购买啦。',
      '先收藏小店吧，明天再来挑选你喜欢的商品。',
    ],
    contact_messages: ['店主暂时休息啦。可以先添加联系方式留言，咨询商品或订单，营业后会尽快回复。'],
  },
}

// Only self-hosted images: storefront requests never send visitors to an image service.
export function safeSupportImage(value: unknown): string {
  const path = String(value || '').trim()
  return /^\/(?:uploads|storefront)\/[\w./-]+\.(?:png|jpe?g|webp|gif)$/i.test(path)
    && !path.includes('..') ? path : ''
}

export function readSupportState(raw: unknown, mode: 'open' | 'sampling'): SupportState {
  const state = raw && typeof raw === 'object' ? raw as Partial<SupportState> : {}
  const images = Array.isArray(state.images) ? state.images.map(safeSupportImage).filter(Boolean).slice(0, 20) : []
  const messages = Array.isArray(state.messages)
    ? state.messages.filter((item): item is string => typeof item === 'string').map(item => item.trim()).filter(Boolean).slice(0, 30)
    : []
  const contactMessages = Array.isArray(state.contact_messages)
    ? state.contact_messages.filter((item): item is string => typeof item === 'string')
      .map(item => [...item.trim()].slice(0, 180).join('')).filter(Boolean).slice(0, 10)
    : supportDefaults[mode].contact_messages
  return {
    images: images.length ? images : supportDefaults[mode].images,
    messages: messages.length ? messages : supportDefaults[mode].messages,
    contact_messages: contactMessages,
  }
}

export function supportMessagePool(state: SupportState): SupportMessage[] {
  return [
    ...state.messages.map(text => ({ text, showContact: false })),
    ...state.contact_messages.map(text => ({ text, showContact: true })),
  ]
}

export function differentIndex(length: number, previous: number, random = Math.random()): number {
  if (length <= 1) return 0
  const previousValid = Number.isInteger(previous) && previous >= 0 && previous < length
  const choice = Math.min(length - (previousValid ? 2 : 1), Math.floor(random * (length - (previousValid ? 1 : 0))))
  return previousValid && choice >= previous ? choice + 1 : choice
}

export function clampCharacterPosition(x: number, y: number, width: number, height: number, viewportWidth: number, viewportHeight: number) {
  return {
    x: Math.max(12, Math.min(x, viewportWidth - width - 12)),
    y: Math.max(82, Math.min(y, viewportHeight - height - 20)),
  }
}

// The handle is the anchor: changing or closing the bubble must not move the feet.
export function clampCharacterAnchor(x: number, y: number, width: number, height: number, bubbleHeight: number, viewportWidth: number, viewportHeight: number) {
  return {
    x: Math.max(12, Math.min(x, viewportWidth - width - 12)),
    y: Math.max(82 + bubbleHeight, Math.min(y, viewportHeight - height - 20)),
  }
}
