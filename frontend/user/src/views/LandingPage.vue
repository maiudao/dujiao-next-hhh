<template>
  <div class="landing-page" :class="{ 'landing-dark': theme === 'dark' }">
    <div class="landing-shell">
      <main class="landing-layout">
        <section class="landing-story" aria-labelledby="landing-title">
          <div class="story-content" :class="{ 'story-content-random': !appStore.isResellerTenant }">
          <div class="story-badges">
            <span v-if="content.badgePrimary" class="story-badge story-badge-primary">
              <Store :size="14" aria-hidden="true" />
              {{ content.badgePrimary }}
            </span>
            <span class="story-badge story-badge-secondary" :class="{ 'story-badge-closed': samplingMode }" role="status">
              <span class="live-dot" aria-hidden="true"></span>
              {{ samplingMode ? businessStatus.closed : businessStatus.open }}
            </span>
          </div>

          <h1 id="landing-title" class="landing-title" :class="{ 'landing-title-random': !appStore.isResellerTenant }">
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
              v-if="!userAuthStore.isAuthenticated"
              type="button"
              class="button-secondary"
              @click="followLink(content.secondaryUrl)"
            >
              {{ content.secondaryLabel || '登录或注册' }}
              <ArrowRight :size="16" aria-hidden="true" />
            </button>
            <button
              v-else-if="userAuthStore.isAuthenticated"
              type="button"
              class="button-secondary button-secondary-authenticated"
              aria-label="已登录"
              @click="router.push('/me')"
            >
              已登录
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
          </div>
        </section>

        <section id="products" class="product-panel" aria-labelledby="products-title">
          <div class="product-panel-header">
            <div>
              <p class="panel-kicker">{{ copy.catalogKicker }}</p>
              <h2 id="products-title">{{ copy.catalogTitle }}</h2>
            </div>
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

          <div v-else-if="products.length" class="product-grid" tabindex="0" role="region" aria-labelledby="products-title">
            <RouterLink
              v-for="product in products"
              :key="product.id || product.slug"
              :to="`/products/${encodeURIComponent(product.slug)}`"
              class="product-option"
              :class="{ 'product-option-sold-out': isSoldOut(product), 'product-option-paused': samplingMode }"
              :aria-label="copy.viewProduct(getLocalizedText(product.title))"
            >
              <span class="product-option-top">
                <span v-if="product.category?.name" class="product-category">
                  <img v-if="product.category.icon && !failedCategoryIcons[product.category.id]"
                    :src="getImageUrl(product.category.icon)" alt="" class="product-category-icon"
                    loading="lazy" @error="failedCategoryIcons[product.category.id] = true" />
                  <Folder v-else class="product-category-icon" aria-hidden="true" />
                  <span class="product-category-name">{{ getLocalizedText(product.category.name) }}</span>
                </span>
                <span
                  class="stock-badge"
                  :class="stockClass(product)"
                >{{ samplingMode ? copy.samplingClosed : getStockStatusLabel(product) }}</span>
              </span>
              <span class="product-title">{{ getLocalizedText(product.title) }}</span>
              <span v-if="product.tags?.length" class="product-tags">
                <span v-for="tag in product.tags" :key="tag" class="product-tag">{{ tag }}</span>
              </span>
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
                  {{ samplingMode || isSoldOut(product) ? copy.viewDetails : copy.selectProduct }}
                  <ArrowRight :size="15" aria-hidden="true" />
                </span>
              </span>
            </RouterLink>
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

<script lang="ts">
// 每次页面加载选一组；两行保持配对，页面内跳转或主题切换不重新抽取。
const headlineVariants = [
  { title: '你需要的那一点便利', accentTitle: '小店替你备着' },
  { title: '生活已经够忙了', accentTitle: '有些事可以省心' },
  { title: '让日常多一点轻松', accentTitle: '从一点便利开始' },
  { title: '你想做的事有很多', accentTitle: '小店帮上一点忙' },
] as const
const selectedHeadline = headlineVariants[Math.floor(Math.random() * headlineVariants.length)]!
</script>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useHead } from '@unhead/vue'
import { useRouter } from 'vue-router'
import {
  ArrowRight,
  Boxes,
  ClipboardCheck,
  Folder,
  MessageCircle,
  PackageCheck,
  PackageSearch,
  RefreshCw,
  ShieldCheck,
  Store,
} from 'lucide-vue-next'
import { productAPI } from '../api'
import { useLocalized, useProductLabels } from '../composables/useProduct'
import { useAnnouncement, type HomeAnnouncement } from '../composables/useAnnouncement'
import { useStorefrontMode } from '../composables/useStorefrontMode'
import AnnouncementModal from '../components/AnnouncementModal.vue'
import { useAppStore } from '../stores/app'
import { useUserAuthStore } from '../stores/userAuth'
import { useTheme } from '../utils/theme'
import { loadAllPages } from '../utils/loadAllPages'
import { getImageUrl } from '../utils/image'

type LandingField = Record<string, string> | undefined

const router = useRouter()
const appStore = useAppStore()
const userAuthStore = useUserAuthStore()
const { samplingMode, samplingNotice } = useStorefrontMode()
const { theme } = useTheme()
const { getLocalizedText, formatPrice, siteCurrency } = useLocalized()
const { isSoldOut, getStockStatusLabel, hasPromotionPrice, getPromotionPriceAmount } = useProductLabels()
const products = ref<any[]>([])
const failedCategoryIcons = ref<Record<number, boolean>>({})
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
const businessStatus = computed(() => {
  if (appStore.locale === 'en-US') return { open: 'The shop is open', closed: 'The shop is closed' }
  if (appStore.locale === 'zh-TW') return { open: '小店正在營業中', closed: '小店已經打烊了' }
  return { open: '小店正在营业中', closed: '小店已经打烊了' }
})
useHead(() => appStore.isResellerTenant ? {} : {
  title: 'HHH 小店｜ChatGPT 代充',
  meta: [
    { name: 'description', content: '第三方人工服务。购买前可查看适用条件、交付流程和售后范围' },
    { property: 'og:title', content: 'HHH 小店｜ChatGPT 代充' },
    { property: 'og:description', content: '第三方人工服务。购买前可查看适用条件、交付流程和售后范围' },
  ],
})
const defaultLanding = computed(() => landingDefaults[appStore.locale as keyof typeof landingDefaults] || landingDefaults['zh-CN'])
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
    title: appStore.isResellerTenant ? localizedValue(config.title, defaults.title) : selectedHeadline.title,
    accentTitle: appStore.isResellerTenant ? localizedValue(config.accent_title, defaults.accentTitle) : selectedHeadline.accentTitle,
    description: localizedValue(config.description, defaults.description),
    primaryLabel: localizedValue(config.primary_label, defaults.primaryLabel),
    primaryUrl: '/products',
    secondaryLabel: localizedValue(config.secondary_label, defaults.secondaryLabel),
    secondaryUrl: '/auth/login?redirect=%2Fproducts',
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

const loadProducts = async () => {
  loading.value = true
  loadError.value = false
  try {
    const allProducts = await loadAllPages<{ sort_order?: number }>(async (page, pageSize) => {
      const response = await productAPI.list({ page, page_size: pageSize })
      const body = response.data
      return {
        items: Array.isArray(body?.data) ? body.data : [],
        totalPages: Number(body?.pagination?.total_page || 0),
      }
    })
    // 首页按后台商品排序权重升序；合并全部分页后再排序，确保跨页顺序一致。
    products.value = allProducts.sort((a, b) => (Number(a.sort_order) || 0) - (Number(b.sort_order) || 0))
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
  --product-bg: #edf3fc;
  --product-line: #c4d4e9;
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
  padding-top: 88px;
  color: var(--ink);
  background: var(--page-bg);
}

.landing-page.landing-dark {
  --page-bg: #0b1220;
  --panel-bg: #131d2e;
  --product-bg: #233149;
  --product-line: #3b506e;
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
  position: relative;
  display: grid;
  grid-template-columns: minmax(240px, 0.73fr) minmax(0, 1.6fr);
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

/* Scale the whole desktop composition, including SVG icons, spacing and controls. */
.story-content {
  min-width: 0;
  zoom: 0.88;
}

.story-content-random {
  container-type: inline-size;
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
  color: var(--green);
}

.story-badge-closed { color: var(--amber); }
.story-badge-closed .live-dot { background: var(--amber); }

.live-dot {
  width: 7px;
  height: 7px;
  background: var(--green);
  border-radius: 50%;
}

.landing-title {
  --headline-max-size: 58px;
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

/* 最长的首句为九个汉字，两句共用字号并随左栏宽度适配。 */
.landing-title.landing-title-random {
  font-size: min(var(--headline-max-size), 10.7cqi);
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

.support-character {
  position: relative;
  z-index: 3;
  grid-column: 1 / -1;
  display: flex;
  align-items: flex-end;
  justify-self: start;
  width: min(370px, 42%);
  min-height: 178px;
  margin: -145px 0 0 39%;
  pointer-events: none;
}

.vault-scope .landing-page {
  padding-top: 0;
}

.support-character-message {
  max-width: 30ch;
  color: var(--ink);
  font-size: 14px;
  line-height: 1.65;
}

.support-character-bubble {
  position: relative;
  z-index: 2;
  max-width: 245px;
  margin: 0 0 92px -22px;
  padding: 14px 17px;
  color: var(--ink);
  text-align: left;
  background: var(--panel-bg);
  border: 1px solid var(--line);
  border-radius: 18px 18px 18px 6px;
  box-shadow: 0 12px 24px rgb(31 63 108 / 12%);
  pointer-events: auto;
  cursor: pointer;
}

.support-character-bubble::after {
  position: absolute;
  right: 24px;
  bottom: -9px;
  width: 16px;
  height: 16px;
  content: '';
  background: var(--panel-bg);
  border-right: 1px solid var(--line);
  border-bottom: 1px solid var(--line);
  transform: rotate(45deg);
}

.support-character-lead {
  display: block;
  margin-bottom: 3px;
  color: var(--blue);
  font-size: 15px;
  font-weight: 800;
}

.support-character-image-button {
  position: relative;
  z-index: 1;
  width: 170px;
  height: 178px;
  padding: 0;
  border: 0;
  background: transparent;
  pointer-events: auto;
  cursor: pointer;
}

.support-character-image {
  align-self: end;
  width: 170px;
  height: 178px;
  object-fit: contain;
  object-position: bottom center;
  filter: drop-shadow(0 8px 8px rgb(31 63 108 / 14%));
}

.support-character-sampling {
  filter: saturate(0.72);
}

.support-character-pressed .support-character-image {
  animation: support-character-squish 220ms ease-out;
}

@keyframes support-character-squish {
  0% { transform: scale(1, 1); }
  45% { transform: scale(1.08, 0.88); }
  100% { transform: scale(1, 1); }
}

.highlight-item {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 48px;
  padding: 0 18px 0 14px;
  color: var(--ink);
  background: var(--panel-bg);
  border: 1px solid var(--line);
  border-radius: 999px;
  font-size: 13px;
  box-shadow: 0 2px 5px rgb(24 48 85 / 5%);
}

.highlight-item :deep(svg) {
  color: var(--blue);
  flex-shrink: 0;
}

.product-panel {
  min-width: 0;
  margin-top: 54px;
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
  margin-bottom: 18px;
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
  min-height: 0;
  padding: 14px;
  color: var(--ink);
  text-align: left;
  background: color-mix(in srgb, var(--product-bg) 48%, var(--panel-bg));
  border: 1px solid var(--product-line);
  border-radius: 16px;
  cursor: pointer;
  text-decoration: none;
  transition: border-color 160ms ease, background-color 160ms ease, transform 160ms ease, box-shadow 160ms ease;
}

.product-option:hover {
  transform: translateY(-2px);
  border-color: color-mix(in srgb, var(--blue) 55%, var(--line));
  box-shadow: 0 8px 18px rgb(31 63 108 / 8%);
}

.product-option:disabled:hover {
  transform: none;
  border-color: var(--product-line);
  box-shadow: none;
}

.product-option-sold-out {
  background: color-mix(in srgb, var(--product-bg) 85%, var(--surface-soft));
}

.product-option-paused {
  cursor: pointer;
}

.product-option-paused .product-action { color: var(--muted); }

.product-option-top,
.product-option-bottom {
  justify-content: space-between;
  gap: 12px;
}

.product-category {
  display: inline-flex;
  align-items: center;
  min-width: 0;
  gap: 6px;
  overflow: hidden;
  color: var(--muted);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.product-category-icon { width: 16px; height: 16px; flex-shrink: 0; object-fit: contain; }
.product-category-name { overflow: hidden; text-overflow: ellipsis; }

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
  display: block;
  margin-top: 9px;
  color: var(--ink);
  font-size: 15px;
  font-weight: 700;
  line-height: 1.45;
  overflow-wrap: anywhere;
}

.product-tags { display: flex; flex-wrap: wrap; gap: 5px; margin-top: 6px; }
.product-tag { max-width: 100%; padding: 2px 7px; border-radius: 5px; color: var(--blue); background: var(--blue-soft); font-size: 11px; font-weight: 600; line-height: 1.6; overflow-wrap: anywhere; }

.product-description {
  display: block;
  margin: 8px 0 10px;
  color: var(--muted);
  font-size: 12px;
  line-height: 1.65;
  overflow-wrap: anywhere;
}

.product-option-bottom {
  align-items: flex-end;
  margin-top: auto;
  padding-top: 10px;
  border-top: 1px solid var(--line);
}

.product-price-block {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 6px;
  min-width: 0;
}

.product-price {
  color: var(--blue);
  font-size: 22px;
  font-weight: 750;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
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
  min-height: 30px;
  padding: 0 8px;
  border-radius: 7px;
  background: var(--blue-soft);
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

@media (min-width: 901px) {
  .landing-page { min-height: 0; }
  .landing-shell { padding-top: 24px; padding-bottom: 24px; }
  .landing-story { align-self: start; justify-content: flex-start; padding-top: 12px; }
  .story-highlights { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 6px; }
  .story-highlights .highlight-item {
    min-width: 0;
    justify-content: center;
    gap: 5px;
    min-height: 36px;
    padding: 0 8px;
    font-size: 11px;
    white-space: nowrap;
  }
  .story-highlights .highlight-item svg { width: 14px; height: 14px; }
  .product-panel {
    display: flex;
    flex-direction: column;
    height: max(480px, calc(100dvh - 119px));
    margin-top: 0;
  }
  .product-panel-header, .catalog-footnote { flex-shrink: 0; }
  .product-grid {
    flex: 1;
    min-height: 0;
    grid-auto-rows: max-content;
    align-content: start;
    overflow-y: auto;
    overscroll-behavior-y: contain;
    scrollbar-gutter: stable;
    scrollbar-width: thin;
    scrollbar-color: var(--product-line) transparent;
    padding: 3px 4px 3px 3px;
    margin: -3px -4px -3px -3px;
  }
  .product-grid:focus-visible { outline: 2px solid var(--blue); outline-offset: 4px; }
  .product-panel > .catalog-state { flex: 1; min-height: 0; }
}

@media (max-width: 1060px) {
  .landing-shell {
    padding: 18px 24px 48px;
  }

  .landing-layout {
    grid-template-columns: minmax(0, 0.74fr) minmax(0, 1.26fr);
    gap: 24px;
  }

  .landing-title {
    --headline-max-size: 46px;
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

  .story-content { zoom: 1; }

  .story-badges {
    margin-bottom: 17px;
  }

  .landing-title {
    --headline-max-size: 42px;
    font-size: 42px;
  }

  .landing-description {
    max-width: 65ch;
    margin-top: 14px;
  }

  .story-actions {
    margin-top: 20px;
  }

  .button-secondary-authenticated {
    display: none;
  }

  .support-character {
    grid-row: 2;
    width: min(100%, 360px);
    min-height: 190px;
    margin: -72px auto 0;
    justify-self: center;
  }

  .support-character-bubble {
    max-width: 220px;
    margin: 0 0 92px -10px;
    font-size: 13px;
  }

  .support-character-image-button,
  .support-character-image {
    width: 158px;
    height: 166px;
  }

  .story-highlights {
    margin-top: 19px;
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 6px;
  }

  .story-highlights .highlight-item { min-width: 0; justify-content: center; gap: 4px; min-height: 34px; padding: 0 6px; font-size: clamp(9px, 2.65vw, 11px); white-space: nowrap; }
  .story-highlights .highlight-item svg { width: 13px; height: 13px; flex-shrink: 0; }

  .product-panel {
    grid-row: 3;
    margin-top: 0;
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
    --headline-max-size: 36px;
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
    padding: 0 14px 0 11px;
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
    min-height: 0;
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
