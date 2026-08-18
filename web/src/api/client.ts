export interface ApiError {
  code: string
  message: string
  status: number
}

export class ApiRequestError extends Error {
  code: string
  status: number
  constructor(code: string, message: string, status: number) {
    super(message)
    this.code = code
    this.status = status
  }
}

let csrfToken = ''
let onUnauthorized: (() => void) | null = null

export function setCsrfToken(token: string) {
  csrfToken = token
}

export function setUnauthorizedHandler(handler: () => void) {
  onUnauthorized = handler
}

function buildUrl(path: string): string {
  return path.startsWith('/api') ? path : `/api/v1${path}`
}

export async function request<T = any>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const method = (options.method || 'GET').toUpperCase()
  const isMutating = !['GET', 'HEAD', 'OPTIONS'].includes(method)

  const headers: Record<string, string> = {
    ...(options.headers as Record<string, string>),
  }
  if (options.body && !headers['Content-Type']) {
    headers['Content-Type'] = 'application/json'
  }
  if (isMutating && csrfToken) {
    headers['X-CSRF-Token'] = csrfToken
  }

  const res = await fetch(buildUrl(path), {
    ...options,
    headers,
    credentials: 'same-origin',
  })

  if (res.status === 401) {
    if (onUnauthorized) onUnauthorized()
    throw new ApiRequestError('unauthorized', 'unauthorized', 401)
  }

  let data: any = null
  const text = await res.text()
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = text
    }
  }

  if (!res.ok) {
    const code = data?.code || 'unknown_error'
    const message = data?.message || 'request failed'
    throw new ApiRequestError(code, message, res.status)
  }

  return data as T
}

export const api = {
  get<T = any>(path: string) {
    return request<T>(path, { method: 'GET' })
  },
  post<T = any>(path: string, body?: any) {
    return request<T>(path, {
      method: 'POST',
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  },
  put<T = any>(path: string, body?: any) {
    return request<T>(path, {
      method: 'PUT',
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  },
  delete<T = any>(path: string, body?: any) {
    return request<T>(path, {
      method: 'DELETE',
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  },
}
