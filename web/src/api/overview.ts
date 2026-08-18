import { api } from './client'

export interface InstanceSummary {
  id: number
  name: string
  connected: boolean
  version: string
  database_count: number
  account_count: number
  protected_count: number
  unprotected_count: number
  last_backup_at: string
  last_backup_status: string
  recent_failed: number
}

export const overviewApi = {
  get() {
    return api.get<InstanceSummary[]>('/overview')
  },
}
