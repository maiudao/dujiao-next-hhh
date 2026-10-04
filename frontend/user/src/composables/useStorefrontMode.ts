import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'

const fallbackLabels = {
  'zh-CN': ['老板睡觉中', '老板在摸鱼', '今日暂停营业'],
  'zh-TW': ['老闆睡覺中', '老闆在摸魚', '今日暫停營業'],
  'en-US': ['Owner is asleep', 'Owner is away', 'Temporarily closed'],
} as const

export function useStorefrontMode() {
  const appStore = useAppStore()
  const { t } = useI18n()
  const samplingMode = computed(() => {
    return !appStore.isResellerTenant && String(appStore.config?.storefront_mode || '').trim().toLowerCase() === 'sampling'
  })
  const samplingLabels = computed(() => {
    const locale = appStore.locale in fallbackLabels ? appStore.locale as keyof typeof fallbackLabels : 'zh-CN'
    const configured = appStore.config?.storefront_sampling_labels
    if (!Array.isArray(configured)) return [...fallbackLabels[locale]]
    return Array.from({ length: 3 }, (_, index) => {
      const entry = configured[index] as Record<string, unknown> | undefined
      const value = String(entry?.[locale] || entry?.['zh-CN'] || entry?.['en-US'] || '').trim()
      return value || fallbackLabels[locale][index]
    })
  })
  const samplingLabelFor = (product: unknown) => {
    const value = typeof product === 'object' && product !== null
      ? String((product as Record<string, unknown>).id ?? (product as Record<string, unknown>).slug ?? '')
      : String(product ?? '')
    let hash = 0
    for (const character of value) hash = (hash * 31 + character.charCodeAt(0)) | 0
    return samplingLabels.value[Math.abs(hash) % samplingLabels.value.length]
  }
  const samplingNotice = computed(() => t('storefront.samplingNotice'))

  return { samplingMode, samplingLabelFor, samplingNotice }
}
