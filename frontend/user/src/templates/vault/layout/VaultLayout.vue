<template>
  <div class="vault-scope">
    <!-- 顶栏 -->
    <header class="sticky top-0 z-50 border-b bg-[color:var(--bg)]">
      <div class="mx-auto flex h-[70px] w-full max-w-[1180px] items-center gap-3 px-4 sm:gap-5 sm:px-6 max-[400px]:gap-1.5 max-[400px]:px-3">
        <RouterLink class="inline-flex min-w-0 items-center gap-2.5 text-foreground" to="/" :title="brandName">
          <img :src="brandLogo || '/dj.svg'" :alt="brandName" class="h-8 max-w-[120px] object-contain sm:max-w-[160px]" />
          <span class="flex min-w-0 flex-col justify-center leading-tight">
            <span class="whitespace-nowrap text-[19px] font-extrabold tracking-[-0.02em] max-[900px]:text-[15px]">{{ brandName }}</span>
            <span class="brand-description mt-0.5 whitespace-nowrap text-[10px] font-medium text-muted-foreground max-[400px]:whitespace-normal max-[400px]:text-[9px]">
              <span v-for="(part, index) in brandDescription.split('，')" :key="index" class="inline-block">{{ part }}{{ index === 0 ? '，' : '' }}</span>
            </span>
          </span>
        </RouterLink>
        <button type="button" class="mobile-contact-entry hidden max-[900px]:inline-flex" @click="contactOpen = true">联系方式</button>

        <nav class="flex gap-0.5 max-[900px]:hidden">
          <button type="button" class="order-1 whitespace-nowrap rounded-full px-3.5 py-2 text-[15px] font-semibold text-muted-foreground hover:bg-secondary hover:text-foreground" @click="contactOpen = true">联系方式</button>
          <template v-for="item in menuItems" :key="item.key">
            <RouterLink
              v-if="item.type === 'route'"
              :to="item.path"
              class="whitespace-nowrap rounded-full px-3.5 py-2 text-[15px] font-semibold text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
              active-class="!bg-primary/10 !text-primary"
              :class="item.key === 'products' ? 'order-0' : 'order-2'"
            >{{ item.label }}</RouterLink>
            <a
              v-else
              :href="item.path"
              :target="item.target"
              rel="noopener noreferrer"
              class="order-2 whitespace-nowrap rounded-full px-3.5 py-2 text-[15px] font-semibold text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
            >{{ item.label }}</a>
          </template>
        </nav>

        <div class="vault-toolbar ml-auto flex items-center gap-2 max-[900px]:gap-1.5">
          <RouterLink v-if="appStore.config?.guest_orders_enabled && !userAuthStore.isAuthenticated" class="grid h-10 w-10 flex-none place-items-center rounded-full bg-secondary text-muted-foreground transition-colors hover:bg-primary/10 hover:text-primary" to="/guest/orders" :aria-label="t('navbar.guestOrders')" :title="t('navbar.guestOrders')"><ClipboardList class="h-[18px] w-[18px]" /></RouterLink>
          <button class="grid h-10 w-10 flex-none place-items-center rounded-full bg-secondary text-muted-foreground transition-colors hover:bg-primary/10 hover:text-primary" type="button" :aria-label="t('resellerConsole.common.toggleTheme')" @click="toggleTheme">
            <Sun v-if="theme === 'dark'" class="h-[18px] w-[18px]" />
            <Moon v-else class="h-[18px] w-[18px]" />
          </button>
          <RouterLink class="relative grid h-10 w-10 flex-none place-items-center rounded-full bg-secondary text-muted-foreground transition-colors hover:bg-primary/10 hover:text-primary" to="/cart" :aria-label="t('navbar.cart')">
            <ShoppingCart class="h-[18px] w-[18px]" />
            <span v-if="cartCount > 0" class="absolute -right-[3px] -top-[3px] grid h-[19px] min-w-[19px] place-items-center rounded-full bg-primary px-[5px] text-[11px] font-bold text-white">{{ cartCount }}</span>
          </RouterLink>

          <!-- 语言切换 -->
          <div v-if="languages.length > 1" class="relative max-[900px]:hidden" ref="langEl">
            <button class="grid h-10 w-10 flex-none place-items-center rounded-full bg-secondary text-muted-foreground transition-colors hover:bg-primary/10 hover:text-primary" type="button" :aria-label="t('navbar.selectLanguage')" @click="toggleLang">
              <Languages class="h-[18px] w-[18px]" />
            </button>
            <div v-if="langOpen" class="absolute right-0 top-[calc(100%+8px)] z-[60] flex min-w-[168px] flex-col gap-0.5 rounded-md border bg-card p-2 shadow-[var(--shadow-lg)]">
              <span class="px-3 pb-0.5 pt-1.5 text-[11px] font-bold uppercase tracking-[0.08em] text-muted-foreground">{{ t('navbar.selectLanguage') }}</span>
              <button v-for="lang in languages" :key="lang.code" class="flex w-full items-center justify-between gap-2.5 rounded-sm px-3 py-2.5 text-left text-sm font-semibold transition-colors hover:bg-secondary hover:text-foreground" :class="appStore.locale === lang.code ? 'text-primary' : 'text-muted-foreground'" @click="changeLanguage(lang.code)">
                {{ lang.name }}
                <span v-if="appStore.locale === lang.code" class="h-[7px] w-[7px] rounded-full bg-primary"></span>
              </button>
            </div>
          </div>

          <!-- 登录 / 个人中心（桌面） -->
          <template v-if="userAuthStore.isAuthenticated">
            <RouterLink class="inline-flex h-10 items-center gap-2 rounded-full border-2 border-hairline-strong px-4 text-[13px] font-bold text-foreground transition-colors hover:border-[color:var(--ink)] max-[900px]:hidden" to="/me">个人中心</RouterLink>
          </template>
          <RouterLink v-else class="inline-flex h-10 items-center gap-2 rounded-full bg-primary px-4 text-[13px] font-bold text-primary-foreground transition-colors hover:bg-primary/90 max-[900px]:hidden" to="/auth/login">未登录</RouterLink>

          <!-- 移动端：更多菜单 -->
          <div class="relative hidden max-[900px]:block" ref="moreEl">
            <button class="grid h-10 w-10 flex-none place-items-center rounded-full bg-secondary text-muted-foreground transition-colors hover:bg-primary/10 hover:text-primary" type="button" :aria-label="t('navbar.more')" :aria-expanded="moreOpen" aria-haspopup="menu" @pointerdown.stop @click.stop="toggleMore">
              <Menu v-if="!moreOpen" class="pointer-events-none h-[18px] w-[18px]" />
              <X v-else class="pointer-events-none h-[18px] w-[18px]" />
            </button>
            <div v-if="moreOpen" class="absolute right-0 top-[calc(100%+8px)] z-[60] flex min-w-[168px] flex-col gap-0.5 rounded-md border bg-card p-2 shadow-[var(--shadow-lg)]" @click.stop>
              <template v-for="item in menuItems" :key="`m-${item.key}`">
                <RouterLink v-if="item.type === 'route'" :to="item.path" class="flex w-full items-center gap-2.5 rounded-sm px-3 py-2.5 text-sm font-semibold text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground" @click="moreOpen = false">{{ item.label }}</RouterLink>
                <a v-else :href="item.path" :target="item.target" rel="noopener noreferrer" class="flex w-full items-center gap-2.5 rounded-sm px-3 py-2.5 text-sm font-semibold text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground" @click="moreOpen = false">{{ item.label }}</a>
              </template>
              <RouterLink v-if="appStore.config?.guest_orders_enabled && !userAuthStore.isAuthenticated" to="/guest/orders" class="flex w-full items-center gap-2.5 rounded-sm px-3 py-2.5 text-sm font-semibold text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground" @click="moreOpen = false">{{ t('navbar.guestOrders') }}</RouterLink>
              <RouterLink v-if="userAuthStore.isAuthenticated" to="/me" class="flex w-full items-center gap-2.5 rounded-sm px-3 py-2.5 text-sm font-semibold text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground" @click="moreOpen = false">{{ t('navbar.personalCenter') }}</RouterLink>
              <RouterLink v-else to="/auth/login" class="flex w-full items-center gap-2.5 rounded-sm px-3 py-2.5 text-sm font-semibold text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground" @click="moreOpen = false">{{ t('navbar.login') }}</RouterLink>
              <span v-if="languages.length > 1" class="px-3 pb-0.5 pt-1.5 text-[11px] font-bold uppercase tracking-[0.08em] text-muted-foreground">{{ t('navbar.selectLanguage') }}</span>
              <button v-for="lang in languages.length > 1 ? languages : []" :key="`ml-${lang.code}`" class="flex w-full items-center justify-between gap-2.5 rounded-sm px-3 py-2.5 text-left text-sm font-semibold transition-colors hover:bg-secondary hover:text-foreground" :class="appStore.locale === lang.code ? 'text-primary' : 'text-muted-foreground'" @click="changeLanguage(lang.code); moreOpen = false">
                {{ lang.name }}
                <span v-if="appStore.locale === lang.code" class="h-[7px] w-[7px] rounded-full bg-primary"></span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </header>

    <!-- 页面内容 -->
    <main class="flex-1">
      <slot />
    </main>

    <!-- 页脚 -->
    <footer class="mt-[var(--gap-block)] border-t bg-[color:var(--bg-warm)]">
      <div class="mx-auto grid w-full max-w-[1180px] grid-cols-1 gap-[30px] px-6 pb-9 pt-[52px] sm:grid-cols-2 lg:grid-cols-[1.7fr_1fr_1fr]">
        <div>
          <RouterLink class="inline-flex items-center gap-2.5 text-[19px] font-extrabold tracking-[-0.02em] text-foreground" to="/">
            <img :src="brandLogo || '/dj.svg'" :alt="brandName" class="h-8 max-w-[160px] object-contain" />
            <span class="truncate">{{ brandName }}</span>
          </RouterLink>
        </div>
        <div>
          <h4 class="mb-3 text-sm font-bold">{{ t('vault.footer.shop') }}</h4>
          <RouterLink v-if="!isListMode" to="/products" class="flex items-center gap-[7px] py-[5px] text-[14.5px] text-muted-foreground hover:text-primary">{{ t('products.allCategories') }}</RouterLink>
          <RouterLink v-if="noticeEnabled" to="/notice" class="flex items-center gap-[7px] py-[5px] text-[14.5px] text-muted-foreground hover:text-primary">{{ t('nav.notice') }}</RouterLink>
          <RouterLink v-if="blogEnabled" to="/blog" class="flex items-center gap-[7px] py-[5px] text-[14.5px] text-muted-foreground hover:text-primary">{{ t('nav.blog') }}</RouterLink>
          <RouterLink to="/me" class="flex items-center gap-[7px] py-[5px] text-[14.5px] text-muted-foreground hover:text-primary">{{ t('navbar.personalCenter') }}</RouterLink>
        </div>
        <div>
          <h4 class="mb-3 text-sm font-bold">{{ t('vault.footer.legal') }}</h4>
          <RouterLink to="/terms" class="flex items-center gap-[7px] py-[5px] text-[14.5px] text-muted-foreground hover:text-primary">{{ t('footer.terms') }}</RouterLink>
          <RouterLink to="/privacy" class="flex items-center gap-[7px] py-[5px] text-[14.5px] text-muted-foreground hover:text-primary">{{ t('footer.privacy') }}</RouterLink>
          <a v-for="link in footerLinks" :key="link.name" :href="link.url || 'javascript:void(0)'" :target="link.url ? '_blank' : undefined" rel="noopener noreferrer" class="flex items-center gap-[7px] py-[5px] text-[14.5px] text-muted-foreground hover:text-primary">{{ link.name }}</a>
        </div>
      </div>
    </footer>
    <ContactDialog v-model:open="contactOpen" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Moon, Sun, ShoppingCart, Languages, Menu, X, ClipboardList, LayoutGrid,
} from 'lucide-vue-next'
import { useAppStore } from '../../../stores/app'
import ContactDialog from '../../../components/ContactDialog.vue'
import { useCartStore } from '../../../stores/cart'
import { useUserAuthStore } from '../../../stores/userAuth'
import { useNavConfig, type NavItem } from '../../../composables/useNavConfig'
import { useTheme } from '../../../utils/theme'
import { getImageUrl } from '../../../utils/image'
import { getLocalizedText } from '../../../utils/resellerSiteConfig'
// 本地自托管字体（替代 Google Fonts CDN），仅 vault 模板加载
import '@fontsource/rubik/latin-400.css'
import '@fontsource/rubik/latin-500.css'
import '@fontsource/rubik/latin-600.css'
import '@fontsource/rubik/latin-700.css'
import '@fontsource/rubik/latin-800.css'
import '@fontsource/nunito-sans/latin-400.css'
import '@fontsource/nunito-sans/latin-500.css'
import '@fontsource/nunito-sans/latin-600.css'
import '@fontsource/nunito-sans/latin-700.css'
import '../styles/vault.css'

const { t } = useI18n()
const appStore = useAppStore()
const cartStore = useCartStore()
const userAuthStore = useUserAuthStore()
const { theme, toggleTheme } = useTheme()

const langOpen = ref(false)
const moreOpen = ref(false)
const contactOpen = ref(false)
const langEl = ref<HTMLElement | null>(null)
const moreEl = ref<HTMLElement | null>(null)

const brandName = computed(() => String(appStore.config?.brand?.site_name || '').trim() || 'HHH 小店')
const brandLogo = computed(() => {
  const raw = String(appStore.config?.brand?.site_logo || '').trim()
  return raw ? getImageUrl(raw) : ''
})
const brandDescription = computed(() => {
  return '售后有保障，用着更放心'
})

const { isListMode, secondaryNavItems, noticeEnabled, blogEnabled } = useNavConfig()

const menuItems = computed<NavItem[]>(() => {
  const items: NavItem[] = []
  // 列表模式下首页就是商品列表，/products 与首页同页，不再单列一项
  if (!isListMode.value) {
    items.push({ key: 'products', path: '/products', label: t('products.allCategories'), icon: LayoutGrid, type: 'route', target: '_self' })
  }
  // 内置（博客/公告/关于）+ 后台自定义导航项
  items.push(...secondaryNavItems.value)
  return items
})

/** 后台配置的自定义页脚链接 */
const footerLinks = computed(() => {
  const links = appStore.config?.footer_links
  if (!Array.isArray(links)) return []
  return links
    .map((item: { name?: unknown; url?: unknown }) => ({
      name: typeof item?.name === 'string' ? item.name.trim() : getLocalizedText(item?.name as Record<string, string>, appStore.locale),
      url: String(item?.url || '').trim(),
    }))
    .filter((item) => item.name)
})


const cartCount = computed(() => cartStore.totalItems)

const languages = computed(() => [
  { code: 'zh-CN', name: '简体中文' },
  { code: 'zh-TW', name: '繁體中文' },
  { code: 'en-US', name: 'English' },
].filter(lang => (appStore.config?.languages || ['zh-CN']).includes(lang.code)))

const changeLanguage = (code: string) => {
  appStore.setLocale(code)
  langOpen.value = false
}

const toggleLang = () => { langOpen.value = !langOpen.value; moreOpen.value = false }
const toggleMore = () => { moreOpen.value = !moreOpen.value; langOpen.value = false }

const onDocClick = (e: MouseEvent) => {
  const target = e.target as Node
  if (langOpen.value && langEl.value && !langEl.value.contains(target)) langOpen.value = false
  if (moreOpen.value && moreEl.value && !moreEl.value.contains(target)) moreOpen.value = false
}

onMounted(() => {
  document.addEventListener('click', onDocClick)
})
onUnmounted(() => {
  document.removeEventListener('click', onDocClick)
})
</script>

<style scoped>
.mobile-contact-entry { flex: none; align-items: center; justify-content: center; min-height: 30px; padding: 0 9px; border: 1px solid var(--ui-accent-soft); border-radius: 9px; background: var(--ui-accent-soft); color: var(--ui-accent); font-size: 10px; font-weight: 700; white-space: nowrap; transition: background-color 140ms; }
.mobile-contact-entry:hover { background: var(--ui-bg-soft); }
.mobile-contact-entry:focus-visible { outline: 2px solid var(--ui-accent); outline-offset: 3px; }
@media (max-width: 360px) { .vault-toolbar { gap: 3px; } .vault-toolbar > a, .vault-toolbar > button, .vault-toolbar button[aria-haspopup='menu'] { width: 32px; height: 34px; } .mobile-contact-entry { padding-inline: 6px; font-size: 9px; } }
</style>

<style scoped>
@media (max-width: 900px) {
  .vault-toolbar > a,
  .vault-toolbar > button,
  .vault-toolbar > div > button { width: 32px; height: 32px; }
}
</style>
