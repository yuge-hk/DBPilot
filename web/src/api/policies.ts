import { api } from './client'

export interface Schedule {
  type: 'daily' | 'weekly' | 'monthly'
  time: string
  day: number
}

export interface BackupPolicy {
  id: number
  name: string
  instance_id: number
  database_name: string
  enabled: boolean
  schedule: Schedule
  retention_count: number
  globals_enabled: boolean
  storage_targets: number[]
  last_run_at: string
  next_run_at: string
  created_at: string
  updated_at: string
}

export interface PolicyPayload {
  name: string
  instance_id: number
  database_name: string
  enabled: boolean
  schedule: Schedule
  retention_count: number
  globals_enabled: boolean
  storage_targets: number[]
}

export const policiesApi = {
  list() {
    return api.get<BackupPolicy[]>('/backup-policies')
  },
  create(payload: PolicyPayload) {
    return api.post<BackupPolicy>('/backup-policies', payload)
  },
  update(id: number, payload: PolicyPayload) {
    return api.put(`/backup-policies/${id}`, payload)
  },
  remove(id: number) {
    return api.delete(`/backup-policies/${id}`)
  },
}
