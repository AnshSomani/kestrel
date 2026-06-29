import { useState, useEffect, useCallback, useRef } from 'react'

async function apiFetch<T>(url: string): Promise<T> {
  let token = localStorage.getItem('kestrel_access_token');
  
  const makeRequest = async (t: string | null) => {
    return fetch(url, {
      headers: t ? { 'Authorization': `Bearer ${t}` } : {},
    });
  };

  let res = await makeRequest(token);

  // If 401, attempt silent refresh
  if (res.status === 401) {
    try {
      const refreshRes = await fetch('/api/auth/refresh', { method: 'POST' });
      if (refreshRes.ok) {
        const data = await refreshRes.json();
        if (data.access_token) {
          localStorage.setItem('kestrel_access_token', data.access_token);
          token = data.access_token;
          // Retry original request
          res = await makeRequest(token);
        }
      } else {
        // Refresh failed, clear session
        localStorage.removeItem('kestrel_access_token');
        localStorage.removeItem('kestrel_user');
        window.dispatchEvent(new Event('auth:expired'));
      }
    } catch (e) {
      // Ignore network errors on refresh
    }
  }

  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}

interface UseApiResult<T> {
  data: T | null
  loading: boolean
  error: string | null
  refetch: () => void
}

function usePollingApi<T>(url: string, interval: number): UseApiResult<T> {
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const fetchData = useCallback(async () => {
    try {
      const result = await apiFetch<T>(url)
      setData(result)
      setError(null)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }, [url])

  useEffect(() => {
    fetchData()
    if (interval > 0) {
      timerRef.current = setInterval(fetchData, interval)
    }
    return () => {
      if (timerRef.current) clearInterval(timerRef.current)
    }
  }, [fetchData, interval])

  return { data, loading, error, refetch: fetchData }
}

// --- Stats ---
export interface DeliveryItem {
  id: string
  event_type: string
  endpoint_url: string
  status: string
  attempt_count: number
  last_status_code?: number
  last_error?: string
  created_at: string
  delivered_at?: string
}

export interface StatsData {
  total_events: number
  deliveries: Record<string, number>
  queue_depth: number
  active_subscriptions: number
  recent_deliveries: DeliveryItem[]
}

export function useStats(pollInterval = 3000) {
  return usePollingApi<StatsData>('/api/stats', pollInterval)
}

// --- Events ---
export interface EventItem {
  id: string
  type: string
  payload: unknown
  idempotency_key?: string
  created_at: string
}

interface EventsResponse {
  events: EventItem[]
  next_cursor?: string
}

export function useEvents(limit = 20, cursor?: string) {
  const url = cursor
    ? `/api/events?limit=${limit}&cursor=${cursor}`
    : `/api/events?limit=${limit}`
  return usePollingApi<EventsResponse>(url, 0)
}

// --- Subscriptions ---
export interface SubscriptionItem {
  id: string
  endpoint_url: string
  secret: string
  event_types: string[]
  is_active: boolean
  created_at: string
}

interface SubscriptionsResponse {
  subscriptions: SubscriptionItem[]
}

export function useSubscriptions() {
  return usePollingApi<SubscriptionsResponse>('/api/subscriptions', 0)
}

// --- Health ---
export interface HealthData {
  status: string
  postgres: string
  queue_depth: number
}

export function useHealth() {
  return usePollingApi<HealthData>('/health', 5000)
}

// --- Helpers ---
export function timeAgo(dateStr: string): string {
  const now = Date.now()
  const then = new Date(dateStr).getTime()
  const diff = Math.max(0, now - then)
  const seconds = Math.floor(diff / 1000)
  if (seconds < 5) return 'just now'
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

export function formatNumber(n: number): string {
  return n.toLocaleString('en-US')
}

export function truncate(str: string, len: number): string {
  if (str.length <= len) return str
  return str.slice(0, len) + '…'
}
