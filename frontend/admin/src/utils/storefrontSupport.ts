import characterCatalog from '../../../user/public/storefront/characters/catalog.json'

export type SupportMode = 'open' | 'sampling'
export interface SupportState { images: string[]; messages: string[] }
export type StorefrontSupport = Record<SupportMode, SupportState>

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
  })
  return { open: normalize('open'), sampling: normalize('sampling') }
}
