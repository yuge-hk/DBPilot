import { api } from './client'

export interface Task {
  id: number
  type: string
  status: 'queued' | 'running' | 'success' | 'partial_success' | 'failed' | 'cancelled'
  progress: number
  resource_type: string
  resource_id: string
  error_code: string
  error_message: string
  log: string
  created_at: string
  started_at: string
  finished_at: string
}

export const tasksApi = {
  list(limit = 100, offset = 0) {
    return api.get<Task[]>(`/tasks?limit=${limit}&offset=${offset}`)
  },
  get(id: number) {
    return api.get<Task>(`/tasks/${id}`)
  },
}
