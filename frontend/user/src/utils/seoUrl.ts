/** Keep this contract aligned with internal/shared/siteurl/origin.go. */
export function normalizePublicOrigin(raw: string): string {
  const value = raw.trim()
  if (!value) return ''
  const absolute = value.startsWith('//')
    ? `https:${value}`
    : value.includes('://') ? value : `https://${value}`
  if (!/^https?:\/\/[^/?#\\]+\/?$/i.test(absolute)) return ''
  try {
    const url = new URL(absolute)
    if (!['http:', 'https:'].includes(url.protocol) || !url.hostname
      || url.username || url.password || url.search || url.hash
      || url.pathname !== '/' || absolute.includes('?') || absolute.includes('#')) return ''
    const local = ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname)
    if (!local) url.protocol = 'https:'
    return url.origin
  } catch {
    return ''
  }
}

export function absolutePublicUrl(raw: string, origin: string): string {
  if (!raw.trim()) return ''
  try {
    const url = new URL(raw.trim(), origin ? `${origin}/` : undefined)
    return ['http:', 'https:'].includes(url.protocol) && !url.username && !url.password
      ? url.href : ''
  } catch {
    return ''
  }
}

export function isPrivateStorefrontPath(path: string): boolean {
  return /^\/(?:auth|me|cart|checkout|pay|orders|recharge-orders|guest|reseller|affiliate)(?:\/|$)/.test(path)
}
