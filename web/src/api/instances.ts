import { api } from './client'

export interface Instance {
  id: number
  name: string
  host: string
  port: number
  username: string
  ssl_mode: string
  unix_socket: string
  systemd_unit: string
  created_at: string
  updated_at: string
}

export interface InstancePayload {
  name: string
  host: string
  port: number
  username: string
  password: string
  ssl_mode: string
  unix_socket: string
  systemd_unit: string
}

export interface TestConnectionPayload {
  host: string
  port: number
  username: string
  password: string
  ssl_mode: string
  unix_socket: string
}

export interface TestResult {
  ok: boolean
  version: string
  server_address: string
  current_user: string
  database_count: number
  error?: string
}

export interface ToolInfo {
  name: string
  path: string
  version: string
  found: boolean
}

export const instancesApi = {
  list() {
    return api.get<Instance[]>('/instances')
  },
  get(id: number) {
    return api.get<Instance>(`/instances/${id}`)
  },
  create(payload: InstancePayload) {
    return api.post<Instance>('/instances', payload)
  },
  update(id: number, payload: InstancePayload) {
    return api.put<Instance>(`/instances/${id}`, payload)
  },
  remove(id: number) {
    return api.delete(`/instances/${id}`)
  },
  test(payload: TestConnectionPayload) {
    return api.post<TestResult>('/instances/test', payload)
  },
  testById(id: number) {
    return api.post<TestResult>(`/instances/${id}/test`)
  },
  tools(id: number) {
    return api.get<ToolInfo[]>(`/instances/${id}/tools`)
  },
}
