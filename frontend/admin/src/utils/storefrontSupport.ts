import characterCatalog from '../../../user/public/storefront/characters/catalog.json'

export type SupportMode = 'open' | 'sampling'
export interface SupportState { images: string[]; messages: string[]; contact_messages: string[] }
export type StorefrontSupport = Record<SupportMode, SupportState> & { paid_order_message: string; delivered_order_message: string }

export type ContactKey = 'qq' | 'wechat' | 'telegram'
export type StorefrontContacts = Record<ContactKey, { enabled: boolean; value: string }>
export function createStorefrontContacts(raw?: any, legacyTelegram = ''): StorefrontContacts {
  const entry = (key: ContactKey) => ({
    enabled: raw ? raw[key]?.enabled === true : key === 'telegram' && !!legacyTelegram,
    value: typeof raw?.[key]?.value === 'string' ? raw[key].value : (!raw && key === 'telegram' ? legacyTelegram : ''),
  })
  return { qq: entry('qq'), wechat: entry('wechat'), telegram: entry('telegram') }
}

export function createStorefrontSupport(raw?: any): StorefrontSupport {
  const normalize = (mode: SupportMode): SupportState => ({
    images: Array.isArray(raw?.[mode]?.images) && raw[mode].images.length
      ? raw[mode].images.map(String)
      : characterCatalog[mode].map(item => item.path),
    messages: Array.isArray(raw?.[mode]?.messages) && raw[mode].messages.length
      ? raw[mode].messages.map(String)
      : mode === 'open'
        ? ['购买前后有什么不清楚的，都可以和客服保持联系哦 (^_^)', '想了解商品或库存？下单前可以先和客服聊聊。', '付款后记得查看订单进度，有售后问题也可以联系店主。']
        : ['明天再来吧，营业时间是每天9：00到12：00。', '店主正在休息，营业后就能继续购买啦。', '先收藏小店吧，明天再来挑选你喜欢的商品。'],
    contact_messages: Array.isArray(raw?.[mode]?.contact_messages)
      ? raw[mode].contact_messages.filter((item: unknown): item is string => typeof item === 'string').slice(0, 10)
      : mode === 'open'
        ? ['想了解商品、购买流程或售后？可以添加店主的联系方式，先聊清楚再下单。']
        : ['店主暂时休息啦。可以先添加联系方式留言，咨询商品或订单，营业后会尽快回复。'],
  })
  return {
    open: normalize('open'), sampling: normalize('sampling'),
    paid_order_message: typeof raw?.paid_order_message === 'string' && raw.paid_order_message.trim()
      ? raw.paid_order_message
      : '订单已付款。可以点击顶部栏的联系方式，联系店主确认处理进度；咨询时请附上订单号。打烊期间回复可能稍晚，请留意开店时间。',
    delivered_order_message: typeof raw?.delivered_order_message === 'string' && raw.delivered_order_message.trim()
      ? raw.delivered_order_message
      : '订单已交付成功，请查看本页“订单交付”里的“交付结果”，那里是店主发送给你的内容。使用方法请查看下方“使用说明”，有疑问可以联系店主。',
  }
}
