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
    const labels = samplingLabels.value
    return labels[Math.abs(hash) % labels.length] ?? labels[0] ?? '今日暂停营业'
  }
  const samplingNotice = computed(() => t('storefront.samplingNotice'))
  const closedPaymentLabel = '已打烊，暂不能支付'
  const refreshBeforePayment = async () => {
    if (!await appStore.loadConfig(true)) throw new Error('暂时无法确认营业状态，请检查网络后重试。')
    if (samplingMode.value) throw new Error(samplingNotice.value)
  }

  return { samplingMode, samplingLabelFor, samplingNotice, closedPaymentLabel, refreshBeforePayment }
}
