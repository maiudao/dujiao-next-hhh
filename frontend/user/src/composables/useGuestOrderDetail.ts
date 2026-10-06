import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { guestOrderAPI } from '../api'
import { debounceAsync } from '../utils/debounce'
import { clearGuestOrderAuth, loadGuestOrderAuth, saveGuestOrderAuth } from '../utils/guestOrderAuth'
import { resolveGuestOrderDetailViewState } from '../utils/guestOrderDetailState'
import { useOrderDisplayHelpers } from './useOrderDisplayHelpers'
import { usePaidOrderSupport } from './usePaidOrderSupport'
import { useOrderRefresh } from './useOrderRefresh'
import { useStorefrontMode } from './useStorefrontMode'

/**
 * 游客订单详情逻辑（classic + vault 共用）。
 */
export function useGuestOrderDetail() {
  const route = useRoute()
  const router = useRouter()
  const { t } = useI18n()

  const loading = ref(true)
  const order = ref<any>(null)
  usePaidOrderSupport(order)
  const { samplingMode, closedPaymentLabel } = useStorefrontMode()
  const authError = ref('')
  const auth = ref({
    email: '',
    order_password: '',
  })
  const fulfillmentDownloading = ref(false)

  const helpers = useOrderDisplayHelpers(order)

  const handleDownloadFulfillment = async (orderNo: string) => {
    if (fulfillmentDownloading.value) return
    fulfillmentDownloading.value = true
    try {
      const res = await guestOrderAPI.downloadFulfillment(orderNo, {
        email: auth.value.email,
        order_password: auth.value.order_password,
      })
      const blob = new Blob([res.data], { type: 'text/plain; charset=utf-8' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `fulfillment-${orderNo}.txt`
      a.click()
      URL.revokeObjectURL(url)
    } catch {} finally {
      fulfillmentDownloading.value = false
    }
  }

  const loadSavedAuth = () => {
    auth.value = loadGuestOrderAuth()
  }

  const hasAuth = computed(() => Boolean(auth.value.email && auth.value.order_password))
  const showAuthForm = computed(() => !hasAuth.value || authError.value !== '')
  const viewState = computed(() => resolveGuestOrderDetailViewState({
    loading: loading.value,
    order: order.value,
    showAuthForm: showAuthForm.value,
  }))

  let orderRequest = 0
  let refreshing = false
  const loadOrder = async (silent = false) => {
    if (silent && refreshing) return
    refreshing = true
    const request = ++orderRequest
    const orderNo = String(route.params.order_no || '').trim()
    if (!silent) loading.value = true
    try {
      if (!hasAuth.value) {
        order.value = null
        authError.value = t('guestOrderDetail.authRequired')
        return
      }
      const response = await guestOrderAPI.detail(String(route.params.order_no || '').trim(), {
        email: auth.value.email,
        order_password: auth.value.order_password,
      })
      if (request !== orderRequest || orderNo !== route.params.order_no) return
      order.value = response.data.data
      authError.value = ''
    } catch (error) {
      if (!silent && request === orderRequest) {
        order.value = null
        authError.value = t('guestOrderDetail.authInvalid')
      }
    } finally {
      if (request === orderRequest) { loading.value = false; refreshing = false }
    }
  }

  const debouncedLoadOrder = debounceAsync(() => loadOrder(), 300)
  useOrderRefresh(() => { if (hasAuth.value && !loading.value && route.params.order_no) void loadOrder(true) })
  watch(() => route.params.order_no, () => { order.value = null; void loadOrder() })

  const persistAuth = () => {
    saveGuestOrderAuth({
      email: auth.value.email,
      order_password: auth.value.order_password,
    })
  }

  const handleAuthSubmit = async () => {
    authError.value = ''
    if (!hasAuth.value) {
      authError.value = t('guestOrderDetail.authRequired')
      return
    }
    persistAuth()
    loading.value = true
    await debouncedLoadOrder()
  }

  const clearAuth = () => {
    clearGuestOrderAuth()
    auth.value = { email: '', order_password: '' }
    order.value = null
    authError.value = t('guestOrderDetail.authRequired')
  }

  onMounted(() => {
    if (!route.params.order_no) {
      router.push('/guest/orders')
      return
    }
    loadSavedAuth()
    loadOrder()
  })

  onUnmounted(() => {
    orderRequest++
    debouncedLoadOrder.cancel()
  })

  return {
    samplingMode, closedPaymentLabel,
    loading,
    order,
    authError,
    auth,
    showAuthForm,
    viewState,
    handleAuthSubmit,
    clearAuth,
    fulfillmentDownloading,
    handleDownloadFulfillment,
    ...helpers,
  }
}
