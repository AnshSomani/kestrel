import { useState } from 'react'
import { useEvents, timeAgo } from '../hooks/useApi'
import './Events.css'

export default function Events() {
  const [page, setPage] = useState(1)
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const { data, loading } = useEvents(20, page)

  return (
    <div className="events-page animate-fade-in">
      <div className="events-header">
        <h1>Events</h1>
        <span className="events-subtitle">Browse ingested webhook events</span>
      </div>

      <div className="events-table-wrap glass-card">
        {loading ? (
          <div className="events-loading">Loading events...</div>
        ) : !data || data.events.length === 0 ? (
          <div className="events-empty">
            <span className="events-empty-icon">⚡</span>
            <p>No events yet. Send a POST to /api/events to get started.</p>
          </div>
        ) : (
          <>
            <table className="events-table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Type</th>
                  <th>Idempotency Key</th>
                  <th>Created</th>
                </tr>
              </thead>
              <tbody>
                {data.events.map((event) => (
                  <>
                    <tr
                      key={event.id}
                      className={`events-row ${expandedId === event.id ? 'events-row--expanded' : ''}`}
                      onClick={() =>
                        setExpandedId(expandedId === event.id ? null : event.id)
                      }
                    >
                      <td className="mono">{event.id.slice(0, 8)}…</td>
                      <td>
                        <span className="event-type-badge">{event.type}</span>
                      </td>
                      <td className="mono text-muted">
                        {event.idempotency_key
                          ? event.idempotency_key.slice(0, 12) + '…'
                          : '—'}
                      </td>
                      <td className="text-muted" title={event.created_at}>
                        {timeAgo(event.created_at)}
                      </td>
                    </tr>
                    {expandedId === event.id && (
                      <tr key={`${event.id}-detail`} className="events-detail-row">
                        <td colSpan={4}>
                          <div className="events-detail">
                            <div className="detail-section">
                              <span className="detail-label">Full ID</span>
                              <code className="mono">{event.id}</code>
                            </div>
                            <div className="detail-section">
                              <span className="detail-label">Payload</span>
                              <pre className="detail-payload mono">
                                {JSON.stringify(event.payload, null, 2)}
                              </pre>
                            </div>
                          </div>
                        </td>
                      </tr>
                    )}
                  </>
                ))}
              </tbody>
            </table>

            <div className="events-pagination" style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '0.5rem', marginTop: '2rem' }}>
              <button
                className="pagination-btn"
                onClick={() => setPage(p => Math.max(1, p - 1))}
                disabled={data.current_page === 1}
              >
                ← Prev
              </button>
              
              {Array.from({ length: data.total_pages }, (_, i) => i + 1).map(p => {
                // Show first, last, and +/- 2 from current
                if (p === 1 || p === data.total_pages || (p >= data.current_page - 2 && p <= data.current_page + 2)) {
                  return (
                    <button
                      key={p}
                      className="pagination-btn"
                      onClick={() => setPage(p)}
                      style={p === data.current_page ? { fontWeight: 'bold', color: 'var(--primary)', borderColor: 'var(--primary)' } : {}}
                    >
                      {p}
                    </button>
                  )
                } else if (p === data.current_page - 3 || p === data.current_page + 3) {
                  return <span key={`ell-${p}`} style={{ color: 'var(--text-muted)' }}>...</span>
                }
                return null;
              })}

              <button
                className="pagination-btn"
                onClick={() => setPage(p => Math.min(data.total_pages, p + 1))}
                disabled={data.current_page === data.total_pages}
              >
                Next →
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
