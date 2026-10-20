import { ref } from 'vue'

const TEA_THEME_KEY = 'dujiao_theme'

type Theme = 'light' | 'dark'

declare global {
    interface Window {
        HuihuaDisplayTheme?: { readonly theme: Theme; setTheme: (theme: Theme) => void; sync: () => void }
    }
}

const getSystemTheme = (): Theme => {
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

const getSavedTheme = (): Theme | null => {
    if (window.HuihuaDisplayTheme) return window.HuihuaDisplayTheme.theme
    try {
        const value = localStorage.getItem(TEA_THEME_KEY)
        return value === 'light' || value === 'dark' ? value : null
    } catch { return null }
}

const theme = ref<Theme>(getSavedTheme() || getSystemTheme())

const applyTheme = (newTheme: Theme) => {
    const root = document.documentElement
    if (newTheme === 'dark') {
        root.classList.add('dark')
    } else {
        root.classList.remove('dark')
    }
    root.style.colorScheme = newTheme
}

applyTheme(theme.value)
window.addEventListener('huihua:theme-change', (event) => {
    theme.value = (event as CustomEvent<{ theme: Theme }>).detail.theme
    applyTheme(theme.value)
})

export const useTheme = () => {
    const toggleTheme = () => {
        theme.value = theme.value === 'dark' ? 'light' : 'dark'
        if (window.HuihuaDisplayTheme) window.HuihuaDisplayTheme.setTheme(theme.value)
        else {
            try { localStorage.setItem(TEA_THEME_KEY, theme.value) } catch {}
        }
        applyTheme(theme.value)
    }

    return {
        theme,
        toggleTheme
    }
}
