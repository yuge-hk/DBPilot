import { api } from './client'

export interface BackupRecord {
  id: number
  instance_id: number
  database: string
  backup_type: 'database' | 'globals'
  format: string
  status: string
  started_at: string
  finished_at: string
  duration_ms: number
  original_size: number
  final_size: number
  sha256: string
  pg_dump_version: string
  postgres_version: string
  error_code: string
  error_message: string
  created_by: string
  manifest_path: string
  created_at: string
}

export interface BackupTargetEntry {
  id: number
  backup_id: number
  storage_id: number
  status: string
  object_path: string
  size: number
  error_code: string
  error_message: string
}

export const backupsApi = {
  list(params?: { instance_id?: number; database?: string }) {
    const q = new URLSearchParams()
    if (params?.instance_id) q.set('instance_id', String(params.instance_id))
    if (params?.database) q.set('database', params.database)
    const qs = q.toString()
    return api.get<BackupRecord[]>(`/backups${qs ? `?${qs}` : ''}`)
  },
  get(id: number) {
    return api.get<{ backup: BackupRecord; targets: BackupTargetEntry[] }>(`/backups/${id}`)
  },
  create(payload: { instance_id: number; database: string; storage_ids: number[] }) {
    return api.post<{ id: number }>('/backups', payload)
  },
  createGlobals(payload: { instance_id: number; storage_ids: number[] }) {
    return api.post<{ id: number }>('/backups/globals', payload)
  },
  remove(id: number) {
    return api.delete(`/backups/${id}`)
  },
}
