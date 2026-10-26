import { createI18n } from 'vue-i18n'
// 默认语言（兜底语言）静态打包，其余语言按需动态加载
import zhCN from './locales/zh-CN.json'

const supportedLocales = ['zh-CN']

// 非默认语言的懒加载器：切换语言时才下载对应语言包 chunk
const localeLoaders: Record<string, () => Promise<{ default: Record<string, unknown> }>> = {
    'zh-TW': () => import('./locales/zh-TW.json'),
    'en-US': () => import('./locales/en-US.json'),
}

const loadedLocales = new Set(['zh-CN'])

export function detectLocale(): string {
    return 'zh-CN'
}

const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    fallbackLocale: 'zh-CN',
    // 键断言为 string，避免 locale 类型被收窄为 "zh-CN" 字面量（其余语言在运行时动态注入）
    messages: { 'zh-CN': zhCN } as Record<string, typeof zhCN>,
})

async function loadLocaleMessages(locale: string): Promise<void> {
    if (loadedLocales.has(locale)) return
    const loader = localeLoaders[locale]
    if (!loader) return
    const messages = await loader()
    i18n.global.setLocaleMessage(locale, (messages.default ?? messages) as any)
    loadedLocales.add(locale)
}

// 记录最新一次切换请求，防止快速连续切换时慢加载的旧请求覆盖新选择
let pendingLocale = ''

// 切换语言：先确保语言包加载完成再更新 locale，避免闪现缺失文案。
// 语言包加载失败时保持当前语言不变，避免整页文案退化为 key。
export async function setI18nLocale(locale: string): Promise<void> {
    if (!supportedLocales.includes(locale)) return
    pendingLocale = locale
    try {
        await loadLocaleMessages(locale)
    } catch (error) {
        console.error(`Failed to load locale messages: ${locale}`, error)
        return
    }
    if (pendingLocale !== locale) return
    i18n.global.locale.value = locale
}

export default i18n
