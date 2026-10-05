export const copyText = async (value: string): Promise<void> => {
  if (navigator?.clipboard?.writeText) {
    try { await navigator.clipboard.writeText(value); return } catch { /* Try the local fallback. */ }
  }
  const textarea = document.createElement('textarea')
  textarea.value = value
  textarea.style.position = 'fixed'
  textarea.style.opacity = '0'
  const active = document.activeElement as HTMLElement | null
  ;(document.querySelector('[role="dialog"]') || document.body).appendChild(textarea)
  textarea.focus()
  textarea.select()
  try {
    if (!document.execCommand('copy')) throw new Error('Clipboard unavailable')
  } finally {
    textarea.remove()
    active?.focus({ preventScroll: true })
  }
}
