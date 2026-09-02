import { useCallback, useEffect, useState } from 'react'
import { transport } from '../api'

/** Fetches a server list endpoint and exposes reload. Used by the settings
 * tabs that bind to REST data (credentials/schedules/skill-hub/mcp-hub). */
export function useServerList<T>(path: string): {
  items: T[]
  loading: boolean
  error: string | null
  reload: () => void
} {
  const [items, setItems] = useState<T[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const reload = useCallback(() => {
    setLoading(true)
    setError(null)
    transport
      .request<T[]>('GET', path)
      .then((data) => {
        setItems(Array.isArray(data) ? data : [])
        setLoading(false)
      })
      .catch((err) => {
        setError(err instanceof Error ? err.message : String(err))
        setLoading(false)
      })
  }, [path])

  useEffect(() => {
    reload()
  }, [reload])

  return { items, loading, error, reload }
}

/** Fetches a single server resource; null when the endpoint is unavailable. */
export function useServerResource<T>(path: string): {
  data: T | null
  loading: boolean
  error: string | null
  reload: () => void
} {
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const reload = useCallback(() => {
    setLoading(true)
    setError(null)
    transport
      .request<T>('GET', path)
      .then((d) => {
        setData(d)
        setLoading(false)
      })
      .catch((err) => {
        setError(err instanceof Error ? err.message : String(err))
        setLoading(false)
      })
  }, [path])

  useEffect(() => {
    reload()
  }, [reload])

  return { data, loading, error, reload }
}

/** Shared server-list row renderer: name + status + optional action button. */
export function RowList<T extends { name?: string }>({
  items,
  loading,
  error,
  statusOf,
  action,
  empty = '无数据',
}: {
  items: T[]
  loading: boolean
  error: string | null
  statusOf: (item: T) => string
  action?: (item: T) => React.ReactNode
  empty?: string
}) {
  if (loading) return <div className="appearance-row"><div className="note">加载中…</div></div>
  if (error) return <div className="appearance-row"><div className="note" style={{ color: 'var(--err)' }}>{error}</div></div>
  if (items.length === 0) return <div className="appearance-row"><div className="note">{empty}</div></div>
  return (
    <>
      {items.map((item) => (
        <div key={item.name ?? ''} className="cred-row">
          <span className="name">{item.name}</span>
          <span className="st">{statusOf(item)}</span>
          {action?.(item)}
        </div>
      ))}
    </>
  )
}
