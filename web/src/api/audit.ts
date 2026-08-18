import { api } from './client'

export interface AuditEntry {
  id: number
  actor: string
  action: string
  resource_type: string
  resource_id: string
  result: string
  remote_ip: string
  detail: string
  created_at: string
}

export const auditApi = {
  list(limit = 200, offset = 0) {
    return api.get<AuditEntry[]>(`/audit?limit=${limit}&offset=${offset}`)
  },
}
