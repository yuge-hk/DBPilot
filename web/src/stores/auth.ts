import { defineStore } from 'pinia'
import { api, setCsrfToken } from '@/api/client'

interface AuthState {
  initialized: boolean | null
  username: string
  csrfToken: string
  loading: boolean
}

export const useAuthStore = defineStore('auth', {
  state: (): AuthState => ({
    initialized: null,
    username: '',
    csrfToken: '',
    loading: false,
  }),
  getters: {
    isAuthenticated: (state) => !!state.username,
  },
  actions: {
    async checkStatus() {
      const res = await api.get<{ initialized: boolean }>('/auth/status')
      this.initialized = res.initialized
    },
    async fetchMe() {
      try {
        const res = await api.get<{ username: string; csrf_token: string }>('/auth/me')
        this.username = res.username
        this.csrfToken = res.csrf_token
        setCsrfToken(res.csrf_token)
        return true
      } catch {
        return false
      }
    },
    async setup(username: string, password: string) {
      const res = await api.post<{ username: string; csrf_token: string }>('/auth/setup', {
        username,
        password,
      })
      this.username = res.username
      this.csrfToken = res.csrf_token
      setCsrfToken(res.csrf_token)
      this.initialized = true
    },
    async login(username: string, password: string) {
      const res = await api.post<{ username: string; csrf_token: string }>('/auth/login', {
        username,
        password,
      })
      this.username = res.username
      this.csrfToken = res.csrf_token
      setCsrfToken(res.csrf_token)
    },
    async logout() {
      try {
        await api.post('/auth/logout')
      } catch {
        // ignore
      }
      this.username = ''
      this.csrfToken = ''
      setCsrfToken('')
    },
  },
})
