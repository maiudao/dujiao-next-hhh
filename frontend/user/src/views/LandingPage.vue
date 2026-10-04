<template>
  <div class="landing-page" :class="{ 'landing-dark': theme === 'dark' }">
    <div class="landing-shell">
      <header v-if="!isVaultTemplate" class="landing-header">
        <RouterLink to="/" class="store-brand" :aria-label="storeName">
          <span class="store-mark"><Store :size="20" aria-hidden="true" /></span>
          <span class="store-name">{{ storeName }}</span>
        </RouterLink>

        <div class="header-controls">
          <label class="locale-picker">
            <span class="sr-only">{{ copy.language }}</span>
            <select :value="appStore.locale" @change="changeLocale">
              <option value="zh-CN">简体中文</option>
              <option value="zh-TW">繁體中文</option>
              <option value="en-US">English</option>
            </select>
          </label>
          <button
            type="button"
            class="theme-toggle"
            :aria-label="theme === 'dark' ? copy.switchLight : copy.switchDark"
            :title="theme === 'dark' ? copy.switchLight : copy.switchDark"
            @click="toggleTheme"
          >
            <Sun v-if="theme === 'dark'" :size="18" aria-hidden="true" />
            <Moon v-else :size="18" aria-hidden="true" />
          </button>
        </div>
      </header>

      <main class="landing-layout">
        <section class="landing-story" aria-labelledby="landing-title">
          <div class="story-badges">
            <span v-if="content.badgePrimary" class="story-badge story-badge-primary">
              <Store :size="14" aria-hidden="true" />
              {{ content.badgePrimary }}
            </span>
            <span v-if="content.badgeSecondary" class="story-badge story-badge-secondary">
              <span class="live-dot" aria-hidden="true"></span>
              {{ content.badgeSecondary }}
            </span>
          </div>

          <h1 id="landing-title" class="landing-title">
            <span>{{ content.title }}</span>
            <span class="landing-title-accent">{{ content.accentTitle }}</span>
          </h1>
          <p class="landing-description">{{ content.description }}</p>

          <p v-if="!loading && !loadError" class="catalog-summary" aria-live="polite">
            <span>{{ copy.productCount(products.length) }}</span>
            <span class="summary-separator" aria-hidden="true">·</span>
            <span>{{ copy.availableCount(availableCount) }}</span>
            <template v-if="lowestPrice">
              <span class="summary-separator" aria-hidden="true">·</span>
              <span>{{ copy.startsAt }} <strong>{{ lowestPrice }}</strong></span>
            </template>
          </p>

          <div class="story-actions">
            <button type="button" class="button-primary" @click="followLink(content.primaryUrl)">
              {{ content.primaryLabel }}
              <ArrowRight :size="17" aria-hidden="true" />
            </button>
            <button
              v-if="content.secondaryLabel && content.secondaryUrl && !userAuthStore.isAuthenticated"
              type="button"
              class="button-secondary"
              @click="followLink(content.secondaryUrl)"
            >
              {{ content.secondaryLabel }}
              <ArrowRight :size="16" aria-hidden="true" />
            </button>
          </div>

          <div v-if="visibleHighlights.length" class="story-highlights">
            <div v-for="(highlight, index) in visibleHighlights" :key="index" class="highlight-item">
              <PackageCheck v-if="index === 0" :size="17" aria-hidden="true" />
              <Boxes v-else-if="index === 1" :size="17" aria-hidden="true" />
              <ClipboardCheck v-else :size="17" aria-hidden="true" />
              <span>{{ highlight }}</span>
            </div>
          </div>
        </section>

        <section id="products" class="product-panel" aria-labelledby="products-title">
          <div class="product-panel-header">
            <div>
              <p class="panel-kicker">{{ copy.catalogKicker }}</p>
              <h2 id="products-title">{{ copy.catalogTitle }}</h2>
              <p class="panel-description">{{ copy.catalogDescription }}</p>
            </div>
            <span class="panel-icon"><PackageSearch :size="20" aria-hidden="true" /></span>
          </div>

          <div v-if="loading" class="catalog-state" role="status">
            <span class="loading-spinner" aria-hidden="true"></span>
            <span>{{ copy.loading }}</span>
          </div>

          <div v-else-if="loadError" class="catalog-state catalog-error" role="alert">
            <p>{{ copy.loadError }}</p>
            <button type="button" class="retry-button" @click="loadProducts">
              <RefreshCw :size="15" aria-hidden="true" />
              {{ copy.retry }}
            </button>
          </div>

          <div v-else-if="products.length" class="product-grid">
            <button
              v-for="product in products"
              :key="product.id || product.slug"
              type="button"
              class="product-option"
              :class="{ 'product-option-sold-out': isSoldOut(product), 'product-option-paused': samplingMode }"
              :aria-label="samplingMode ? samplingLabelFor(product) : copy.viewProduct(getLocalizedText(product.title))"
              :disabled="samplingMode"
              :aria-disabled="samplingMode"
              @click="openProduct(product.slug)"
            >
              <span class="product-option-top">
                <span v-if="product.category?.name" class="product-category">
                  {{ getLocalizedText(product.category.name) }}
                </span>
                <span
                  class="stock-badge"
                  :class="stockClass(product)"
                >{{ samplingMode ? samplingLabelFor(product) : getStockStatusLabel(product) }}</span>
              </span>
              <span class="product-title">{{ getLocalizedText(product.title) }}</span>
              <span v-if="getLocalizedText(product.description)" class="product-description">
                {{ getLocalizedText(product.description) }}
              </span>
              <span class="product-option-bottom">
                <span class="product-price-block">
                  <span class="product-price">{{ getDisplayPrice(product) }}</span>
                  <span v-if="hasPromotionPrice(product)" class="product-original-price">
                    {{ formatPrice(product.price_amount, siteCurrency) }}
                  </span>
                </span>
                <span class="product-action">
                  {{ samplingMode ? copy.samplingClosed : (isSoldOut(product) ? copy.viewDetails : copy.selectProduct) }}
                  <ArrowRight :size="15" aria-hidden="true" />
                </span>
              </span>
            </button>
          </div>

          <div v-else class="catalog-state">
            <PackageSearch :size="26" aria-hidden="true" />
            <span>{{ copy.emptyCatalog }}</span>
          </div>

          <p class="catalog-footnote">
            <ShieldCheck :size="16" aria-hidden="true" />
            {{ samplingMode ? samplingNotice : copy.orderCheckNote }}
          </p>
        </section>
      </main>
    </div>

    <a
      v-if="content.supportUrl && content.supportLabel"
      class="support-link"
      :href="content.supportUrl"
      :target="isExternalLink(content.supportUrl) ? '_blank' : undefined"
      :rel="isExternalLink(content.supportUrl) ? 'noopener noreferrer' : undefined"
    >
      <MessageCircle :size="18" aria-hidden="true" />
      <span>{{ content.supportLabel }}</span>
    </a>

    <AnnouncementModal
      v-if="activeAnnouncement"
      :announcement="activeAnnouncement"
      :visible="announcementVisible"
      @update:visible="announcementVisible = $event"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import {
  ArrowRight,
  Boxes,
  ClipboardCheck,
  MessageCircle,
  Moon,
  PackageCheck,
  PackageSearch,
  RefreshCw,
  ShieldCheck,
  Store,
  Sun,
} from 'lucide-vue-next'
import { productAPI } from '../api'
import { useLocalized, useProductLabels } from '../composables/useProduct'
import { useAnnouncement, type HomeAnnouncement } from '../composables/useAnnouncement'
import { useStorefrontMode } from '../composables/useStorefrontMode'
import AnnouncementModal from '../components/AnnouncementModal.vue'
import { useAppStore } from '../stores/app'
import { useUserAuthStore } from '../stores/userAuth'
import { getActiveTemplate } from '../templates/registry'
import { useTheme } from '../utils/theme'
import { loadAllPages } from '../utils/loadAllPages'

type LandingField = Record<string, string> | undefined

const router = useRouter()
const appStore = useAppStore()
const userAuthStore = useUserAuthStore()
const isVaultTemplate = computed(() => getActiveTemplate() === 'vault')
const { samplingMode, samplingLabelFor, samplingNotice } = useStorefrontMode()
const { theme, toggleTheme } = useTheme()
const { getLocalizedText, formatPrice, siteCurrency } = useLocalized()
const { isSoldOut, getStockStatusLabel, hasPromotionPrice, getPromotionPriceAmount } = useProductLabels()
const products = ref<any[]>([])
const loading = ref(true)
const loadError = ref(false)
const { shouldShow } = useAnnouncement()
const announcementVisible = ref(false)
const activeAnnouncement = computed(() => (appStore.config?.announcement as HomeAnnouncement | undefined) || null)

const localeCopy = {
  'zh-CN': {
    language: '语言', switchLight: '切换到浅色主题', switchDark: '切换到深色主题',
    catalogKicker: '商品与价格', catalogTitle: '选择商品', catalogDescription: '查看商品详情、库存与价格，选择后进入原有下单流程。',
    loading: '正在读取在售商品…', loadError: '商品暂时无法加载。', retry: '重新加载', emptyCatalog: '当前没有可展示的商品。',
    selectProduct: '选择并查看', viewDetails: '查看详情', samplingClosed: '暂停中', orderCheckNote: '下单前可在商品与订单确认页核对库存、金额及支付方式。',
    startsAt: '起', productCount: (count: number) => `${count} 款在售商品`, availableCount: (count: number) => `${count} 款当前有库存`,
    viewProduct: (name: string) => `查看 ${name} 商品详情`,
  },
  'zh-TW': {
    language: '語言', switchLight: '切換至淺色主題', switchDark: '切換至深色主題',
    catalogKicker: '商品與價格', catalogTitle: '選擇商品', catalogDescription: '查看商品詳情、庫存與價格，選擇後進入既有下單流程。',
    loading: '正在讀取上架商品…', loadError: '商品暫時無法載入。', retry: '重新載入', emptyCatalog: '目前沒有可展示的商品。',
    selectProduct: '選擇並查看', viewDetails: '查看詳情', samplingClosed: '暫停中', orderCheckNote: '下單前可在商品與訂單確認頁核對庫存、金額及付款方式。',
    startsAt: '起', productCount: (count: number) => `${count} 款上架商品`, availableCount: (count: number) => `${count} 款目前有庫存`,
    viewProduct: (name: string) => `查看 ${name} 商品詳情`,
  },
  'en-US': {
    language: 'Language', switchLight: 'Switch to light theme', switchDark: 'Switch to dark theme',
    catalogKicker: 'Products and prices', catalogTitle: 'Choose a product', catalogDescription: 'Review product details, stock, and price before continuing to checkout.',
    loading: 'Loading available products…', loadError: 'Products could not be loaded.', retry: 'Try again', emptyCatalog: 'There are no products to show right now.',
    selectProduct: 'View product', viewDetails: 'View details', samplingClosed: 'Paused', orderCheckNote: 'Review stock, amount, and payment method on the product and order confirmation pages.',
    startsAt: 'From', productCount: (count: number) => `${count} listed products`, availableCount: (count: number) => `${count} currently in stock`,
    viewProduct: (name: string) => `View ${name} details`,
  },
} as const

const landingDefaults = {
  'zh-CN': {
    badgePrimary: '店铺精选', badgeSecondary: '商品库存实时展示', title: '选择适合你的服务', accentTitle: '从这里开始',
    description: '浏览店铺当前可购买的商品，查看价格与库存后再下单。', primaryLabel: '查看商品套餐',
    secondaryLabel: '登录或注册', supportLabel: '在线咨询', highlights: ['商品信息清晰', '实时查看库存', '订单进度可查'],
  },
  'zh-TW': {
    badgePrimary: '店鋪精選', badgeSecondary: '商品庫存即時展示', title: '選擇適合你的服務', accentTitle: '從這裡開始',
    description: '瀏覽店鋪目前可購買的商品，確認價格與庫存後再下單。', primaryLabel: '查看商品方案',
    secondaryLabel: '登入或註冊', supportLabel: '線上諮詢', highlights: ['商品資訊清晰', '即時查看庫存', '訂單進度可查'],
  },
  'en-US': {
    badgePrimary: 'Selected for you', badgeSecondary: 'Live product availability', title: 'Find a service that fits', accentTitle: 'Start here',
    description: 'Browse available products and review their price and stock before ordering.', primaryLabel: 'Browse products',
    secondaryLabel: 'Sign in or register', supportLabel: 'Contact support', highlights: ['Clear product details', 'Live stock status', 'Track order progress'],
  },
} as const

const copy = computed(() => localeCopy[appStore.locale as keyof typeof localeCopy] || localeCopy['zh-CN'])
const defaultLanding = computed(() => landingDefaults[appStore.locale as keyof typeof landingDefaults] || landingDefaults['zh-CN'])
const storeName = computed(() => String(appStore.config?.brand?.site_name || 'huihuahui.xyz').trim())
const landingConfig = computed(() => appStore.config?.home_landing || {})

const localizedValue = (field: LandingField, fallback: string) => {
  if (!field) return fallback
  for (const locale of [appStore.locale, 'zh-CN', 'en-US']) {
    if (Object.prototype.hasOwnProperty.call(field, locale)) return String(field[locale] ?? '')
  }
  return fallback
}

const safeLink = (raw: unknown) => {
  const value = String(raw || '').trim()
  if (!value) return ''
  if (value.startsWith('#')) return value
  if (value.startsWith('/') && !value.startsWith('//')) return value
  try {
    const parsed = new URL(value)
    if (parsed.protocol === 'https:' || parsed.protocol === 'http:') return value
    if (parsed.protocol === 'mailto:' || parsed.protocol === 'tel:') return value
  } catch {
    return ''
  }
  return ''
}

const content = computed(() => {
  const config = landingConfig.value
  const defaults = defaultLanding.value
  const highlights = Array.isArray(config.highlights) ? config.highlights : defaults.highlights
  return {
    badgePrimary: localizedValue(config.badge_primary, defaults.badgePrimary),
    badgeSecondary: localizedValue(config.badge_secondary, defaults.badgeSecondary),
    title: localizedValue(config.title, defaults.title),
    accentTitle: localizedValue(config.accent_title, defaults.accentTitle),
    description: localizedValue(config.description, defaults.description),
    primaryLabel: localizedValue(config.primary_label, defaults.primaryLabel),
    primaryUrl: safeLink(config.primary_url ?? '#products') || '#products',
    secondaryLabel: localizedValue(config.secondary_label, defaults.secondaryLabel),
    secondaryUrl: safeLink(config.secondary_url ?? '/login?returnTo=%2Fproducts'),
    supportLabel: localizedValue(config.support_label, defaults.supportLabel),
    supportUrl: safeLink(config.support_url),
    highlights: highlights.map((item: LandingField, index: number) => localizedValue(item, defaults.highlights[index] || '')).filter(Boolean),
  }
})

const visibleHighlights = computed(() => content.value.highlights.slice(0, 3))
const availableCount = computed(() => products.value.filter((product) => !isSoldOut(product)).length)
const lowestPrice = computed(() => {
  const prices = products.value
    .map((product) => Number(hasPromotionPrice(product) ? getPromotionPriceAmount(product) : product.price_amount))
    .filter((price) => Number.isFinite(price) && price >= 0)
  return prices.length ? formatPrice(Math.min(...prices), siteCurrency.value) : ''
})

const getDisplayPrice = (product: any) => formatPrice(
  hasPromotionPrice(product) ? getPromotionPriceAmount(product) : product.price_amount,
  siteCurrency.value,
)

const stockClass = (product: any) => {
  if (samplingMode.value) return 'stock-paused'
  if (isSoldOut(product)) return 'stock-sold-out'
  if (product.stock_status === 'low_stock') return 'stock-low'
  return 'stock-available'
}

const isExternalLink = (href: string) => {
  if (!/^https?:\/\//i.test(href)) return false
  try {
    return new URL(href).origin !== window.location.origin
  } catch {
    return false
  }
}

const followLink = (raw: unknown) => {
  const href = safeLink(raw) || '#products'
  if (href.startsWith('#')) {
    document.querySelector(href)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    return
  }
  if (href.startsWith('/')) {
    void router.push(href)
    return
  }
  if (/^https?:\/\//i.test(href)) {
    window.open(href, '_blank', 'noopener,noreferrer')
    return
  }
  window.location.assign(href)
}

const openProduct = (slug: string) => {
  if (samplingMode.value) return
  if (slug) void router.push(`/products/${encodeURIComponent(slug)}`)
}

const changeLocale = (event: Event) => {
  const value = (event.target as HTMLSelectElement).value
  if (['zh-CN', 'zh-TW', 'en-US'].includes(value)) appStore.setLocale(value)
}

const loadProducts = async () => {
  loading.value = true
  loadError.value = false
  try {
    products.value = await loadAllPages(async (page, pageSize) => {
      const response = await productAPI.list({ page, page_size: pageSize })
      const body = response.data
      return {
        items: Array.isArray(body?.data) ? body.data : [],
        totalPages: Number(body?.pagination?.total_page || 0),
      }
    })
  } catch (error) {
    loadError.value = true
    console.error('Failed to load landing page products:', error)
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  if (activeAnnouncement.value && shouldShow(activeAnnouncement.value)) {
    announcementVisible.value = true
  }
  await loadProducts()
})
</script>

<style scoped>
.landing-page {
  --page-bg: #f5f8fd;
  --panel-bg: #ffffff;
  --surface-soft: #f6f9fd;
  --ink: #101827;
  --muted: #65758e;
  --line: #dce5f1;
  --blue: #1d5fe3;
  --blue-soft: #edf4ff;
  --green: #07835f;
  --green-soft: #eaf8f2;
  --amber: #a56a00;
  --amber-soft: #fff6e4;
  min-height: 100vh;
  color: var(--ink);
  background: var(--page-bg);
}

.landing-page.landing-dark {
  --page-bg: #0b1220;
  --panel-bg: #131d2e;
  --surface-soft: #192538;
  --ink: #edf3fc;
  --muted: #a0aec2;
  --line: #2d3b50;
  --blue: #82adff;
  --blue-soft: #1b3155;
  --green: #59d6ac;
  --green-soft: #173a33;
  --amber: #ffd077;
  --amber-soft: #3b3020;
}

.landing-shell {
  width: min(100%, 1180px);
  margin: 0 auto;
  padding: 24px 24px 52px;
}

.landing-header,
.store-brand,
.header-controls,
.story-badges,
.story-actions,
.story-highlights,
.product-option-top,
.product-option-bottom,
.product-action,
.catalog-footnote,
.support-link {
  display: flex;
  align-items: center;
}

.landing-header {
  justify-content: space-between;
  min-height: 48px;
  margin-bottom: 30px;
}

.store-brand {
  gap: 11px;
  min-width: 0;
  color: var(--ink);
  text-decoration: none;
}

.store-mark,
.panel-icon {
  display: grid;
  flex: none;
  place-items: center;
  width: 40px;
  height: 40px;
  color: var(--blue);
  background: var(--blue-soft);
  border: 1px solid var(--line);
  border-radius: 13px;
}

.store-name {
  overflow: hidden;
  font-size: 16px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.header-controls {
  gap: 8px;
}

.locale-picker select,
.theme-toggle {
  height: 38px;
  color: var(--ink);
  background: var(--panel-bg);
  border: 1px solid var(--line);
  border-radius: 10px;
}

.locale-picker select {
  max-width: 140px;
  padding: 0 10px;
  font-size: 13px;
}

.theme-toggle {
  display: grid;
  place-items: center;
  width: 40px;
  cursor: pointer;
}

.locale-picker select:focus-visible,
.theme-toggle:focus-visible,
.landing-page button:focus-visible,
.support-link:focus-visible,
.store-brand:focus-visible {
  outline: 3px solid color-mix(in srgb, var(--blue) 45%, transparent);
  outline-offset: 2px;
}

.landing-layout {
  display: grid;
  grid-template-columns: minmax(0, 0.9fr) minmax(0, 1.1fr);
  align-items: stretch;
  gap: 34px;
}

.landing-story {
  display: flex;
  flex-direction: column;
  justify-content: center;
  min-width: 0;
  padding: 28px 6px 32px 0;
}

.story-badges {
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 24px;
}

.story-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 27px;
  padding: 4px 10px;
  color: var(--blue);
  background: var(--panel-bg);
  border: 1px solid var(--line);
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
}

.story-badge-secondary {
  color: var(--muted);
}

.live-dot {
  width: 7px;
  height: 7px;
  background: var(--green);
  border-radius: 50%;
}

.landing-title {
  display: flex;
  flex-direction: column;
  margin: 0;
  color: var(--ink);
  font-size: 58px;
  font-weight: 760;
  line-height: 1.12;
  overflow-wrap: anywhere;
}

.landing-title-accent {
  color: var(--blue);
}

.landing-description {
  max-width: 56ch;
  margin: 22px 0 0;
  color: var(--muted);
  font-size: 17px;
  line-height: 1.8;
  white-space: pre-line;
}

.catalog-summary {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  margin: 20px 0 0;
  color: var(--muted);
  font-size: 13px;
}

.catalog-summary strong {
  color: var(--ink);
  font-size: 15px;
}

.summary-separator {
  color: #9eb0c9;
}

.story-actions {
  flex-wrap: wrap;
  gap: 11px;
  margin-top: 28px;
}

.button-primary,
.button-secondary,
.retry-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 9px;
  min-height: 47px;
  padding: 0 19px;
  border-radius: 13px;
  font-size: 14px;
  font-weight: 650;
  cursor: pointer;
  transition: background-color 160ms ease, border-color 160ms ease, transform 160ms ease;
}

.button-primary {
  color: #ffffff;
  background: #1d5fe3;
  border: 1px solid #1d5fe3;
  box-shadow: 0 8px 16px rgb(29 95 227 / 18%);
}

.landing-dark .button-primary {
  color: #0b1220;
  background: #82adff;
  border-color: #82adff;
}

.button-primary:hover,
.button-secondary:hover,
.retry-button:hover {
  transform: translateY(-1px);
}

.button-primary:hover {
  background: #164fca;
}

.button-secondary {
  color: var(--ink);
  background: var(--panel-bg);
  border: 1px solid var(--line);
}

.story-highlights {
  flex-wrap: wrap;
  gap: 9px;
  margin-top: 27px;
}

.highlight-item {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 48px;
  padding: 0 13px;
  color: var(--ink);
  background: var(--panel-bg);
  border: 1px solid var(--line);
  border-radius: 999px;
  font-size: 13px;
  box-shadow: 0 2px 5px rgb(24 48 85 / 5%);
}

.highlight-item :deep(svg) {
  color: var(--blue);
}

.product-panel {
  min-width: 0;
  padding: 28px;
  background: var(--panel-bg);
  border: 1px solid var(--line);
  border-radius: 20px;
  box-shadow: 0 18px 42px rgb(31 63 108 / 8%);
  scroll-margin-top: 20px;
}

.product-panel-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 22px;
}

.panel-kicker {
  margin: 0 0 7px;
  color: var(--blue);
  font-size: 12px;
  font-weight: 700;
}

.product-panel-header h2 {
  margin: 0;
  color: var(--ink);
  font-size: 27px;
  font-weight: 730;
  line-height: 1.25;
}

.panel-description {
  margin: 8px 0 0;
  color: var(--muted);
  font-size: 14px;
  line-height: 1.6;
}

.panel-icon {
  width: 42px;
  height: 42px;
  border: 0;
  border-radius: 50%;
}

.product-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.product-option {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 154px;
  padding: 16px;
  color: var(--ink);
  text-align: left;
  background: var(--panel-bg);
  border: 1px solid var(--line);
  border-radius: 16px;
  cursor: pointer;
  transition: border-color 160ms ease, background-color 160ms ease, transform 160ms ease, box-shadow 160ms ease;
}

.product-option:hover {
  transform: translateY(-2px);
  border-color: color-mix(in srgb, var(--blue) 55%, var(--line));
  box-shadow: 0 8px 18px rgb(31 63 108 / 8%);
}

.product-option:disabled:hover {
  transform: none;
  border-color: var(--line);
  box-shadow: none;
}

.product-option-sold-out {
  background: var(--surface-soft);
}

.product-option-paused {
  cursor: not-allowed;
  filter: grayscale(1);
  opacity: 0.78;
}

.product-option-top,
.product-option-bottom {
  justify-content: space-between;
  gap: 12px;
}

.product-category {
  overflow: hidden;
  color: var(--muted);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.stock-badge {
  flex: none;
  padding: 4px 8px;
  border: 1px solid transparent;
  border-radius: 999px;
  font-size: 10px;
  line-height: 1;
}

.stock-available {
  color: var(--green);
  background: var(--green-soft);
  border-color: color-mix(in srgb, var(--green) 24%, transparent);
}

.stock-low {
  color: var(--amber);
  background: var(--amber-soft);
  border-color: color-mix(in srgb, var(--amber) 24%, transparent);
}

.stock-sold-out {
  color: #b83d48;
  background: #fff0f0;
  border-color: #f4c9ce;
}

.stock-paused {
  color: var(--muted);
  background: var(--surface-soft);
  border-color: var(--line);
}

.landing-dark .stock-sold-out {
  color: #ff9ea6;
  background: #44252d;
  border-color: #663b45;
}

.product-title {
  display: -webkit-box;
  overflow: hidden;
  margin-top: 9px;
  color: var(--ink);
  font-size: 16px;
  font-weight: 700;
  line-height: 1.4;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.product-description {
  display: -webkit-box;
  overflow: hidden;
  margin-top: 5px;
  color: var(--muted);
  font-size: 12px;
  line-height: 1.5;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.product-option-bottom {
  align-items: flex-end;
  margin-top: auto;
  padding-top: 15px;
}

.product-price-block {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 6px;
  min-width: 0;
}

.product-price {
  color: var(--ink);
  font-size: 22px;
  font-weight: 750;
  font-variant-numeric: tabular-nums;
  overflow-wrap: anywhere;
}

.product-original-price {
  color: var(--muted);
  font-size: 11px;
  text-decoration: line-through;
}

.product-action {
  flex: none;
  gap: 5px;
  color: var(--blue);
  font-size: 11px;
  font-weight: 700;
}

.catalog-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  min-height: 250px;
  color: var(--muted);
  text-align: center;
  border: 1px dashed var(--line);
  border-radius: 15px;
}

.catalog-error p {
  margin: 0;
}

.loading-spinner {
  width: 27px;
  height: 27px;
  border: 3px solid var(--line);
  border-top-color: var(--blue);
  border-radius: 50%;
  animation: spin 800ms linear infinite;
}

.retry-button {
  min-height: 38px;
  color: var(--ink);
  background: var(--panel-bg);
  border: 1px solid var(--line);
  font-size: 13px;
}

.catalog-footnote {
  gap: 9px;
  margin: 18px 0 0;
  padding: 12px 14px;
  color: var(--muted);
  background: var(--surface-soft);
  border-radius: 999px;
  font-size: 11px;
  line-height: 1.5;
}

.catalog-footnote :deep(svg) {
  flex: none;
  color: var(--green);
}

.support-link {
  position: fixed;
  right: 22px;
  bottom: 20px;
  z-index: 40;
  gap: 9px;
  min-height: 46px;
  padding: 0 17px;
  color: #ffffff;
  background: #1d5fe3;
  border-radius: 999px;
  box-shadow: 0 8px 22px rgb(19 76 182 / 27%);
  font-size: 13px;
  font-weight: 650;
  text-decoration: none;
}

.landing-dark .support-link {
  color: #0b1220;
  background: #82adff;
}

.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  clip-path: inset(50%);
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

@media (max-width: 1060px) {
  .landing-shell {
    padding: 18px 24px 48px;
  }

  .landing-layout {
    grid-template-columns: minmax(0, 0.84fr) minmax(0, 1.16fr);
    gap: 24px;
  }

  .landing-title {
    font-size: 46px;
  }

  .landing-description {
    font-size: 15px;
  }

  .product-panel {
    padding: 22px;
  }

  .product-option {
    padding: 13px;
  }
}

@media (max-width: 780px) {
  .landing-shell {
    padding: 14px 18px 40px;
  }

  .landing-header {
    margin-bottom: 12px;
  }

  .landing-layout {
    grid-template-columns: minmax(0, 1fr);
    gap: 16px;
  }

  .landing-story {
    padding: 25px 0 16px;
  }

  .story-badges {
    margin-bottom: 17px;
  }

  .landing-title {
    font-size: 42px;
  }

  .landing-description {
    max-width: 65ch;
    margin-top: 14px;
  }

  .story-actions {
    margin-top: 20px;
  }

  .story-highlights {
    margin-top: 19px;
  }

  .product-panel {
    padding: 20px;
  }
}

@media (max-width: 520px) {
  .landing-shell {
    padding: 12px 14px 34px;
  }

  .store-mark {
    width: 36px;
    height: 36px;
  }

  .store-name {
    max-width: 54vw;
    font-size: 14px;
  }

  .locale-picker select {
    max-width: 102px;
    padding: 0 6px;
    font-size: 11px;
  }

  .theme-toggle {
    width: 36px;
    height: 36px;
  }

  .landing-title {
    font-size: 36px;
  }

  .landing-description {
    font-size: 14px;
    line-height: 1.7;
  }

  .catalog-summary {
    gap: 7px;
    font-size: 11px;
  }

  .story-actions {
    display: grid;
    grid-template-columns: 1fr;
  }

  .button-primary,
  .button-secondary {
    width: 100%;
  }

  .highlight-item {
    min-height: 40px;
    padding: 0 10px;
    font-size: 11px;
  }

  .product-panel {
    padding: 16px;
    border-radius: 17px;
  }

  .product-panel-header h2 {
    font-size: 23px;
  }

  .panel-description {
    font-size: 12px;
  }

  .panel-icon {
    width: 36px;
    height: 36px;
  }

  .product-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .product-option {
    min-height: 144px;
    padding: 14px;
  }

  .product-price {
    font-size: 20px;
  }

  .catalog-footnote {
    align-items: flex-start;
    border-radius: 12px;
  }

  .support-link {
    right: 14px;
    bottom: 14px;
    min-height: 43px;
    padding: 0 14px;
  }
}
</style>
