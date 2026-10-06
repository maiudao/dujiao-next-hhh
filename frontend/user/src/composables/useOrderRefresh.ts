import { onMounted, onUnmounted } from 'vue'

export function useOrderRefresh(refresh: () => void) {
  const refreshVisible = () => { if (document.visibilityState !== 'hidden') refresh() }
  let timer: ReturnType<typeof setInterval> | undefined
  onMounted(() => {
    timer = setInterval(refreshVisible, 5_000)
    window.addEventListener('focus', refreshVisible)
    document.addEventListener('visibilitychange', refreshVisible)
  })
  onUnmounted(() => {
    clearInterval(timer)
    window.removeEventListener('focus', refreshVisible)
    document.removeEventListener('visibilitychange', refreshVisible)
  })
}
