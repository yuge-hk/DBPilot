import { api } from './client'

export interface StorageTarget {
  id: number
  name: string
  type: 'local' | 'webdav' | 'sftp'
  config: any
  created_at: string
  updated_at: string
}

export interface StoragePayload {
  name: string
  type: 'local' | 'webdav' | 'sftp'
  config: Record<string, any>
  password?: string
}

export const storagesApi = {
  list() {
    return api.get<StorageTarget[]>('/storages')
  },
  create(payload: StoragePayload) {
    return api.post<StorageTarget>('/storages', payload)
  },
  update(id: number, payload: StoragePayload) {
    return api.put(`/storages/${id}`, payload)
  },
  remove(id: number) {
    return api.delete(`/storages/${id}`)
  },
  test(payload: StoragePayload) {
    return api.post<{ ok: boolean; error?: string }>('/storages/test', payload)
  },
}
