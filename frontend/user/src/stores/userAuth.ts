import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { userAuthAPI } from '../api'
import { toast } from '../composables/useToast'

export const useUserAuthStore = defineStore('user-auth', () => {
    const router = useRouter()

    const token = ref<string>(localStorage.getItem('user_token') || '')
    const storedUser = localStorage.getItem('user_profile')
    let parsedUser = null
    try {
        if (storedUser && storedUser !== 'undefined') {
            parsedUser = JSON.parse(storedUser)
        }
    } catch (e) {
        console.error('Failed to parse user profile from localStorage', e)
        localStorage.removeItem('user_profile')
    }
    const user = ref<any>(parsedUser)
    const loading = ref(false)

    // 2FA 挑战中间状态（不持久化到 localStorage）
    const challengeToken = ref<string>('')
    const challengeExpiresAt = ref<string>('')

    const isAuthenticated = computed(() => !!token.value)

    const setToken = (newToken: string) => {
        token.value = newToken
        localStorage.setItem('user_token', newToken)
    }

    const setUser = (newUser: any) => {
        user.value = newUser
        localStorage.setItem('user_profile', JSON.stringify(newUser))
    }

    const clearAuth = () => {
        token.value = ''
        user.value = null
        localStorage.removeItem('user_token')
        localStorage.removeItem('user_profile')
    }

    let federationEnabled: boolean | null = null
    let sessionRequest: Promise<void> | null = null
    let watchingSession = false
    let loggingOut = false
    const sessionSignal = () => document.cookie.split('; ').find(part => part.startsWith('huihua-member-signal=')) || ''

    // Only the host-only HttpOnly authority cookie can restore a login. The
    // shared marker merely asks both open sites to check the server again.
    const syncBrowserSession = (): Promise<void> => {
        if (loggingOut) return Promise.resolve()
        if (sessionRequest) return sessionRequest
        sessionRequest = (async () => {
            const response = await fetch('/api/identity/browser', {
                credentials: 'same-origin', cache: 'no-store',
                headers: { 'X-Huihua-Session': '1' }, signal: AbortSignal.timeout(8000),
            })
            if (response.status === 404) { federationEnabled = false; return }
            if (!response.ok) throw new Error('统一登录状态暂时无法确认，请稍后重试。')
            const state = await response.json()
            if (state.enabled !== true || typeof state.logged_in !== 'boolean') throw new Error('登录状态无效')
            federationEnabled = true
            if (state.logged_in) {
                if (typeof state.token !== 'string' || !state.token || !state.user?.id) throw new Error('登录状态无效')
                setToken(state.token)
                setUser(state.user)
            } else {
                clearAuth()
            }
        })().finally(() => { sessionRequest = null })
        return sessionRequest
    }

    const watchBrowserSession = () => {
        if (watchingSession) return
        watchingSession = true
        let signal = sessionSignal()
        const check = async () => {
            if (document.hidden) return
            try {
                await syncBrowserSession()
                if (federationEnabled && !isAuthenticated.value && router.currentRoute.value.meta.requiresUserAuth) {
                    await router.replace('/auth/login')
                }
            } catch (_) { /* An outage preserves local state; protected APIs still validate the session. */ }
        }
        window.addEventListener('focus', check)
        window.addEventListener('pageshow', check)
        document.addEventListener('visibilitychange', () => { if (!document.hidden) void check() })
        window.setInterval(() => {
            const current = sessionSignal()
            if (current !== signal && !document.hidden) { signal = current; void check() }
        }, 1500)
    }

    const sendVerifyCode = async (payload: any) => {
        loading.value = true
        try {
            await userAuthAPI.sendVerifyCode(payload)
            return true
        } finally {
            loading.value = false
        }
    }

    const register = async (payload: any) => {
        loading.value = true
        try {
            const response = await userAuthAPI.register(payload)
            const { token: accessToken, user: userData } = response.data.data
            if (accessToken) {
                setToken(accessToken)
            }
            if (userData) {
                setUser(userData)
            }
            return true
        } finally {
            loading.value = false
        }
    }

    const clearChallenge = () => {
        challengeToken.value = ''
        challengeExpiresAt.value = ''
    }

    const login = async (payload: any) => {
        loading.value = true
        try {
            const response = await userAuthAPI.login(payload)
            return handleLoginResponse(response.data.data)
        } finally {
            loading.value = false
        }
    }

    const verify2FA = async (payload: { code?: string; recovery_code?: string }) => {
        if (!challengeToken.value) {
            throw new Error('challenge_token_missing')
        }
        loading.value = true
        try {
            const response = await userAuthAPI.verify2FA({
                challenge_token: challengeToken.value,
                ...payload,
            })
            const data = response.data.data || {}
            const { token: accessToken, user: userData } = data
            setToken(accessToken)
            setUser(userData)
            clearChallenge()
            return true
        } finally {
            loading.value = false
        }
    }

    const handleLoginResponse = (data: any): { requiresTotp: boolean } => {
        if (data?.requires_totp) {
            challengeToken.value = data.challenge_token || ''
            challengeExpiresAt.value = data.challenge_expires_at || ''
            return { requiresTotp: true }
        }
        clearChallenge()
        const { token: accessToken, user: userData } = data || {}
        setToken(accessToken)
        setUser(userData)
        return { requiresTotp: false }
    }

    const telegramLogin = async (payload: any) => {
        loading.value = true
        try {
            const response = await userAuthAPI.telegramLogin(payload)
            return handleLoginResponse(response.data.data)
        } finally {
            loading.value = false
        }
    }

    const telegramOidcLogin = async (payload: { code: string; state: string }) => {
        loading.value = true
        try {
            const response = await userAuthAPI.telegramOidcCallback(payload)
            return handleLoginResponse(response.data.data)
        } finally {
            loading.value = false
        }
    }

    const telegramMiniAppLogin = async (initData: string) => {
        loading.value = true
        try {
            const response = await userAuthAPI.telegramMiniAppLogin({ init_data: initData })
            return handleLoginResponse(response.data.data)
        } finally {
            loading.value = false
        }
    }

    const googleLogin = async (credential: string) => {
        loading.value = true
        try {
            const response = await userAuthAPI.googleLogin({ credential })
            return handleLoginResponse(response.data.data)
        } finally {
            loading.value = false
        }
    }

    const googleRedirectLogin = async () => {
        loading.value = true
        try {
            const response = await userAuthAPI.googleRedirectExchange()
            return handleLoginResponse(response.data.data)
        } finally {
            loading.value = false
        }
    }

    const forgotPassword = async (payload: any) => {
        loading.value = true
        try {
            await userAuthAPI.forgotPassword(payload)
            return true
        } finally {
            loading.value = false
        }
    }

    const syncUserProfile = (profile: {
        id?: number
        email?: string
        nickname?: string
        locale?: string
        email_verified_at?: string | null
        email_change_mode?: 'bind_only' | 'change_with_old_and_new'
        password_change_mode?: 'set_without_old' | 'change_with_old'
    }) => {
        const nextUser = { ...(user.value || {}), ...profile }
        setUser(nextUser)
    }

    const logout = async (redirect = '/auth/login') => {
        if (loggingOut) return false
        try {
            // Do not report a successful logout until authority revocation has
            // committed. Waiting also prevents an in-flight restore racing it.
            await syncBrowserSession()
            loggingOut = true
            if (federationEnabled) {
                const response = await fetch('/api/identity/browser/logout', {
                    method: 'POST', credentials: 'same-origin', cache: 'no-store',
                    headers: { 'X-Huihua-Session': '1' }, signal: AbortSignal.timeout(8000),
                })
                if (!response.ok) throw new Error('退出暂时未完成，请稍后重试。')
            }
            clearAuth()
            clearChallenge()
            await router.push(redirect)
            return true
        } catch (_) {
            toast.error('退出暂时未完成，请稍后重试。')
            return false
        } finally {
            loggingOut = false
        }
    }

    return {
        token,
        user,
        loading,
        challengeToken,
        challengeExpiresAt,
        isAuthenticated,
        sendVerifyCode,
        register,
        login,
        verify2FA,
        clearChallenge,
        telegramLogin,
        telegramOidcLogin,
        telegramMiniAppLogin,
        googleLogin,
        googleRedirectLogin,
        forgotPassword,
        syncUserProfile,
        logout,
        syncBrowserSession,
        watchBrowserSession,
    }
})
