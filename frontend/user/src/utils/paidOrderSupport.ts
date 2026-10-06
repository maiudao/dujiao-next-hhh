export const defaultPaidOrderMessage = '订单已付款。可以点击顶部栏的联系方式，联系店主确认处理进度；咨询时请附上订单号。打烊期间回复可能稍晚，请留意开店时间。'
export const defaultDeliveredOrderMessage = '订单已交付成功，请查看本页“订单交付”里的“交付结果”，那里是店主发送给你的内容。使用方法请查看下方“使用说明”，有疑问可以联系店主。'

export function orderSupportPhase(order: unknown): '' | 'paid' | 'delivered' {
  if (!order || typeof order !== 'object') return ''
  const status = String((order as { status?: unknown }).status || '')
  if (['delivered', 'completed'].includes(status)) return 'delivered'
  return ['paid', 'fulfilling', 'partially_delivered'].includes(status) ? 'paid' : ''
}

export function isPaidSupportOrder(order: unknown): boolean {
  return !!orderSupportPhase(order)
}

export function readPaidOrderMessage(raw: unknown, phase: 'paid' | 'delivered' = 'paid'): string {
  return typeof raw === 'string' && raw.trim() ? Array.from(raw.trim()).slice(0, 300).join('')
    : phase === 'delivered' ? defaultDeliveredOrderMessage : defaultPaidOrderMessage
}
