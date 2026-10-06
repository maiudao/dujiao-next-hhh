import { computed, onUnmounted, shallowRef, watch, type Ref } from 'vue'
import { useRoute } from 'vue-router'
import { orderSupportPhase } from '../utils/paidOrderSupport'

const currentOrderPage = shallowRef<{ owner: symbol; path: string; orderNo: string; phase: 'paid' | 'delivered' } | null>(null)

export function usePaidOrderSupport(order: Ref<unknown>) {
  const route = useRoute()
  const owner = Symbol('paid-order-page')
  watch(order, value => {
    const phase = orderSupportPhase(value)
    currentOrderPage.value = phase
      ? { owner, path: route.path, orderNo: String((value as { order_no?: unknown }).order_no || ''), phase }
      : null
  }, { immediate: true })
  onUnmounted(() => {
    if (currentOrderPage.value?.owner === owner) currentOrderPage.value = null
  })
}

export function usePaidOrderPage() {
  const route = useRoute()
  const phase = computed(() => currentOrderPage.value?.path === route.path ? currentOrderPage.value.phase : '')
  const key = computed(() => phase.value ? `${currentOrderPage.value?.orderNo}:${phase.value}` : '')
  return { phase, key }
}
