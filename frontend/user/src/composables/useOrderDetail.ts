import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { userOrderAPI } from '../api'
import { debounceAsync } from '../utils/debounce'
import { useConfirmDialog } from './useConfirmDialog'
import { toast } from './useToast'
import { useOrderDisplayHelpers } from './useOrderDisplayHelpers'
import { usePaidOrderSupport } from './usePaidOrderSupport'
import { useStorefrontMode } from './useStorefrontMode'
import { useOrderRefresh } from './useOrderRefresh'

/**
 * 已登录用户订单详情逻辑（classic + vault 共用）。
 */
export function useOrderDetail() {
  const route = useRoute()
  const router = useRouter()
  const { confirm: showConfirm } = useConfirmDialog()
  const { t } = useI18n()

  const loading = ref(true)
  const order = ref<any>(null)
  usePaidOrderSupport(order)
  const { samplingMode, samplingNotice, closedPaymentLabel } = useStorefrontMode()
  const fulfillmentDownloading = ref(false)

  const helpers = useOrderDisplayHelpers(order)

  const handleDownloadFulfillment = async (orderNo: string) => {
    if (fulfillmentDownloading.value) return
    fulfillmentDownloading.value = true
    try {
      const res = await userOrderAPI.downloadFulfillment(orderNo)
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

  let orderRequest = 0
  let refreshing = false
  const loadOrder = async (silent = false) => {
    if (silent && refreshing) return
    refreshing = true
    const request = ++orderRequest
    const orderNo = String(route.params.order_no || '').trim()
    if (!silent) loading.value = true
    try {
      const response = await userOrderAPI.detail(orderNo, { cache: 'no-store', silentBusinessError: silent })
      if (request !== orderRequest || orderNo !== route.params.order_no) return
      order.value = response.data.data
    } catch (error) {
      if (!silent && request === orderRequest) order.value = null
    } finally {
      if (request === orderRequest) { loading.value = false; refreshing = false }
    }
  }
  useOrderRefresh(() => { if (route.params.order_no && !loading.value) void loadOrder(true) })
  watch(() => route.params.order_no, () => { order.value = null; void loadOrder() })

  const debouncedLoadOrder = debounceAsync(() => loadOrder(), 300)

  const cancelOrder = async () => {
    if (!order.value) return
    const confirmed = await showConfirm({
      title: t('orderDetail.cancel'),
      message: t('orderDetail.cancelConfirm'),
      confirmText: t('common.confirm'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    })
    if (!confirmed) return
    try {
      await userOrderAPI.cancel(order.value.order_no)
      await debouncedLoadOrder()
    } catch {
      toast.error(t('orderDetail.cancelFailed'))
    }
  }

  onMounted(() => {
    if (!route.params.order_no) {
      router.push('/me/orders')
      return
    }
    loadOrder()
  })

  onUnmounted(() => {
    orderRequest++
    debouncedLoadOrder.cancel()
  })

  return {
    samplingMode, samplingNotice, closedPaymentLabel,
    loading,
    order,
    debouncedLoadOrder,
    cancelOrder,
    fulfillmentDownloading,
    handleDownloadFulfillment,
    ...helpers,
  }
}
