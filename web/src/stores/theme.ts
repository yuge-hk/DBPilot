import { defineStore } from 'pinia'

export const useThemeStore = defineStore('theme', {
  state: () => ({
    dark: localStorage.getItem('dbpilot_theme') === 'dark',
  }),
  actions: {
    toggle() {
      this.dark = !this.dark
      localStorage.setItem('dbpilot_theme', this.dark ? 'dark' : 'light')
    },
  },
})
