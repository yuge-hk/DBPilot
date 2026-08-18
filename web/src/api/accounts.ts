import { api } from './client'

export interface Account {
  name: string
  can_login: boolean
  superuser: boolean
  create_db: boolean
  create_role: boolean
  conn_limit: number
  valid_until: string
  owned_dbs: number
  system_role: boolean
}

export interface AccountPayload {
  name?: string
  password?: string
  login?: boolean
  superuser?: boolean
  create_db?: boolean
  create_role?: boolean
  conn_limit?: number
  valid_until?: string
}

export const accountsApi = {
  list(instanceId: number) {
    return api.get<Account[]>(`/instances/${instanceId}/accounts`)
  },
  create(instanceId: number, payload: AccountPayload) {
    return api.post<{ name: string }>(`/instances/${instanceId}/accounts`, payload)
  },
  update(instanceId: number, name: string, payload: AccountPayload) {
    return api.put(`/instances/${instanceId}/accounts/${encodeURIComponent(name)}`, payload)
  },
  remove(instanceId: number, name: string) {
    return api.delete(`/instances/${instanceId}/accounts/${encodeURIComponent(name)}`)
  },
}
