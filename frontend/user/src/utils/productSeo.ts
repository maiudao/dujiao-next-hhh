export const seoText = (raw: unknown): string => String(raw || '').replace(/<[^>]*>/g, ' ').replace(/\s+/g, ' ').trim()

/** Uses public product fields only; does not infer terms or regions from slugs. */
export function productSeo(title: string, tags: string[], explicitTitle: string, description: string, fallback: string) {
  const name = seoText(title)
  const extra = tags.map(seoText).filter((tag) => tag && !name.includes(tag)).slice(0, 2)
  const heading = seoText(explicitTitle) || [name, ...extra].filter(Boolean).join(' · ')
  const summary = seoText(description) || seoText(fallback)
  const text = summary && !summary.startsWith(heading) ? `${heading}：${summary}` : summary || heading
  return { title: heading, description: Array.from(text).slice(0, 160).join('') }
}
