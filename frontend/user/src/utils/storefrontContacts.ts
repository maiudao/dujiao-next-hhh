export const contactKinds = [
  { key: 'qq', label: 'QQ', copyLabel: 'QQ号' },
  { key: 'wechat', label: '微信', copyLabel: '微信号' },
  { key: 'telegram', label: 'Telegram', copyLabel: 'Telegram链接' },
] as const

export function visibleStorefrontContacts(raw: unknown, legacyTelegram = '') {
  const config = raw && typeof raw === 'object' ? raw as Record<string, unknown> : null
  return contactKinds.flatMap(kind => {
    const entry = config?.[kind.key] as { enabled?: unknown; value?: unknown } | undefined
    const value = typeof entry?.value === 'string' ? entry.value.trim() : (!config && kind.key === 'telegram' ? legacyTelegram.trim() : '')
    const enabled = config ? entry?.enabled === true : kind.key === 'telegram'
    return enabled && value ? [{ ...kind, value }] : []
  })
}
