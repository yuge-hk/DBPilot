import { defineStore } from 'pinia'
import { instancesApi, type Instance } from '@/api/instances'

export const useInstanceStore = defineStore('instance', {
  state: () => ({
    instances: [] as Instance[],
    selectedId: Number(localStorage.getItem('dbpilot_instance')) || 0,
    loaded: false,
  }),
  getters: {
    selected: (state) => state.instances.find((i) => i.id === state.selectedId) ?? null,
  },
  actions: {
    async load(force = false) {
      if (this.loaded && !force) return
      this.instances = await instancesApi.list()
      this.loaded = true
      if (!this.selectedId && this.instances.length > 0) {
        this.select(this.instances[0].id)
      }
      if (this.selectedId && !this.instances.some((i) => i.id === this.selectedId)) {
        this.select(this.instances[0]?.id ?? 0)
      }
    },
    select(id: number) {
      this.selectedId = id
      localStorage.setItem('dbpilot_instance', String(id))
    },
  },
})
