/* InferGlow Web UI — REST Transport（浏览器直连 Server） */

const BASE = ''  // 同源，无需前缀

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json', ...init?.headers },
    ...init,
  })
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText)
    throw new Error(`${res.status} ${text}`)
  }
  return res.json() as Promise<T>
}

export async function get<T>(path: string): Promise<T> {
  return request<T>(path)
}

export async function post<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
}

export async function patch<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, {
    method: 'PATCH',
    body: JSON.stringify(body),
  })
}

export async function del<T>(path: string): Promise<T> {
  return request<T>(path, { method: 'DELETE' })
}

/** 检测后端是否可达 */
export async function healthCheck(): Promise<boolean> {
  try {
    const res = await fetch(`${BASE}/health`, { method: 'GET' })
    return res.ok
  } catch {
    return false
  }
}
