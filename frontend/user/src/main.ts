import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createHead } from '@unhead/vue/client'
import './style.css'
import App from './App.vue'
import router, { warmupCommonRoutes } from './router'
import i18n, { detectLocale, setI18nLocale, warmupLocaleMessages } from './i18n'
import { useTelegramMiniAppStore } from './stores/telegramMiniApp'
import { useAppStore } from './stores/app'
import { getActiveTemplate, initializeStorefrontTemplate, initTemplateOverride, loadVaultLayout } from './templates/registry'

// 预览用：?template=vault 持久化激活模板（站长正式切换走站点配置）
initTemplateOverride()

const brandLog = (globalThis as any).console?.log?.bind(console)
brandLog?.(
  '%c Dujiao-Next %c Digital Commerce Platform ',
  'background:#0071e3;color:#fff;padding:4px 8px;border-radius:4px 0 0 4px;font-weight:bold;',
  'background:#1d1d1f;color:#f5f5f7;padding:4px 8px;border-radius:0 4px 4px 0;',
)
brandLog?.('%cGitHub → https://github.com/dujiao-next', 'color:#6e6e73;')

const app = createApp(App)
const head = createHead()
const pinia = createPinia()

app.use(pinia)
app.use(head)
app.use(i18n)

const bootstrap = async () => {
  // 先取得配置，再解析路由；不能先挂载默认 classic 外壳等配置回来后切换。
  const appStore = useAppStore(pinia)
  const [configLoaded] = await Promise.all([
    appStore.loadConfig(),
    useTelegramMiniAppStore(pinia).init(),
    setI18nLocale(detectLocale()),
  ])
  if (!configLoaded || !appStore.config || typeof appStore.config !== 'object' || Array.isArray(appStore.config)) {
    throw new Error('Storefront configuration is unavailable')
  }

  initializeStorefrontTemplate()
  const layoutReady = getActiveTemplate() === 'vault' ? loadVaultLayout() : Promise.resolve()
  app.use(router)
  // router.isReady 同时等待当前页面 chunk；layoutReady 同时等待外壳及其 CSS。
  await Promise.all([router.isReady(), layoutReady])
  app.mount('#app')
  document.documentElement.dataset.storefrontReady = 'true'
  warmupCommonRoutes()
  warmupLocaleMessages()
}

// 配置/资源失败时提供明确的重试入口，不能静默显示原版页面或永久留白。
void bootstrap().catch(() => {
  const target = document.getElementById('app')
  if (!target) return
  const panel = document.createElement('section')
  panel.className = 'storefront-startup-error'
  panel.setAttribute('role', 'alert')
  const heading = document.createElement('h1')
  heading.textContent = '暂时无法打开小店'
  const message = document.createElement('p')
  message.textContent = '网络或页面资源加载失败，请稍后重试。'
  const retry = document.createElement('button')
  retry.type = 'button'
  retry.textContent = '重新加载'
  retry.addEventListener('click', () => window.location.reload())
  panel.append(heading, message, retry)
  target.replaceChildren(panel)
})
