import { api } from './client'

export interface Database {
  name: string
  owner: string
  encoding: string
  collation: string
  size_bytes: number
  conn_count: number
}

export interface CreateDatabasePayload {
  database: string
  account_mode: 'create' | 'existing'
  account: string
  password: string
  permission: 'owner' | 'read_write' | 'read_only'
  remote_access: boolean
  remote_address: string
}

export interface CreateDatabaseResult {
  database: string
  account: string
  host: string
  port: number
  ssl_mode: string
  unix_socket: string
  permission: string
  remote_access: boolean
  remote_address: string
  firewall_warning?: string
}

export const databasesApi = {
  list(instanceId: number) {
    return api.get<Database[]>(`/instances/${instanceId}/databases`)
  },
  create(instanceId: number, payload: CreateDatabasePayload) {
    return api.post<CreateDatabaseResult>(`/instances/${instanceId}/databases`, payload)
  },
  remove(instanceId: number, name: string) {
    return api.delete(`/instances/${instanceId}/databases/${name}`)
  },
}
