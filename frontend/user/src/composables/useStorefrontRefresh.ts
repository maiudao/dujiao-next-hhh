import { onMounted, onUnmounted } from 'vue'
import { useAppStore } from '../stores/app'

export function useStorefrontRefresh() {
  const app = useAppStore()
  const refresh = () => {
    if (document.visibilityState !== 'hidden') void app.loadConfig(true)
  }
  let timer: ReturnType<typeof setInterval> | undefined
  onMounted(() => {
    timer = setInterval(refresh, 15_000)
    window.addEventListener('focus', refresh)
    window.addEventListener('online', refresh)
    document.addEventListener('visibilitychange', refresh)
  })
  onUnmounted(() => {
    clearInterval(timer)
    window.removeEventListener('focus', refresh)
    window.removeEventListener('online', refresh)
    document.removeEventListener('visibilitychange', refresh)
  })
}
