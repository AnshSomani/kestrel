import { useState } from 'react'
import { useSubscriptions, timeAgo } from '../hooks/useApi'
import './Subscriptions.css'

const DEFAULT_EVENTS = [
  'user.signed_up',
  'subscription.upgraded',
  'invoice.payment_failed',
]

export default function Subscriptions() {
  const { data, loading, refetch } = useSubscriptions()
  const [showForm, setShowForm] = useState(false)
  const [endpoint, setEndpoint] = useState('')
  const [secret, setSecret] = useState('')
  
  const [availableEvents, setAvailableEvents] = useState<string[]>(DEFAULT_EVENTS)
  const [selectedEvents, setSelectedEvents] = useState<Set<string>>(new Set(DEFAULT_EVENTS))
  const [customEvent, setCustomEvent] = useState('')
  
  const [submitting, setSubmitting] = useState(false)
  const [errorMsg, setErrorMsg] = useState('')

  const handleEventToggle = (evt: string) => {
    const next = new Set(selectedEvents)
    if (next.has(evt)) next.delete(evt)
    else next.add(evt)
    setSelectedEvents(next)
  }

  const handleAddCustomEvent = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      const val = customEvent.trim()
      if (val && !availableEvents.includes(val)) {
        setAvailableEvents([...availableEvents, val])
        
        const nextSelected = new Set(selectedEvents)
        nextSelected.add(val)
        setSelectedEvents(nextSelected)
      }
      setCustomEvent('')
    }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (selectedEvents.size === 0) {
      setErrorMsg('Please select at least one event type.')
      return
    }
    setSubmitting(true)
    setErrorMsg('')
    try {
      const eventTypes = Array.from(selectedEvents)
      const token = localStorage.getItem('kestrel_access_token')
      const API_BASE = import.meta.env.VITE_API_URL || '';
      const res = await fetch(`${API_BASE}/api/subscriptions`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({
          endpoint_url: endpoint,
          secret: secret,
          event_types: eventTypes
        })
      })
      if (!res.ok) {
        throw new Error('Failed to create subscription')
      }
      setEndpoint('')
      setSecret('')
      setShowForm(false)
      refetch()
    } catch (err: any) {
      setErrorMsg(err.message)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="subs-page animate-fade-in">
      <div className="subs-header">
        <div>
          <h1>Subscriptions</h1>
          <span className="subs-subtitle">Active webhook endpoints</span>
        </div>
        <button className="create-sub-btn" onClick={() => setShowForm(!showForm)}>
          {showForm ? 'Cancel' : '+ New Subscription'}
        </button>
      </div>

      {showForm && (
        <form className="glass-card create-sub-form" onSubmit={handleSubmit}>
          <h3>Create New Subscription</h3>
          {errorMsg && <div className="form-error">{errorMsg}</div>}
          <div className="form-group">
            <label>Endpoint URL</label>
            <input 
              type="url" 
              required 
              value={endpoint} 
              onChange={e => setEndpoint(e.target.value)} 
              placeholder="https://your-api.com/webhooks"
            />
          </div>
          <div className="form-group">
            <label>Secret</label>
            <input 
              type="text" 
              required 
              value={secret} 
              onChange={e => setSecret(e.target.value)} 
              placeholder="Webhook signing secret"
            />
          </div>
          <div className="form-group">
            <label>Event Types</label>
            <div className="event-checkbox-grid">
              {availableEvents.map(evt => (
                <label key={evt} className="event-checkbox-item">
                  <input 
                    type="checkbox" 
                    checked={selectedEvents.has(evt)}
                    onChange={() => handleEventToggle(evt)}
                  />
                  <span className="mono">{evt}</span>
                </label>
              ))}
              <div className="custom-event-input-wrapper">
                <input
                  type="text"
                  placeholder="+ Add custom event (Press Enter)"
                  value={customEvent}
                  onChange={e => setCustomEvent(e.target.value)}
                  onKeyDown={handleAddCustomEvent}
                  className="custom-event-input"
                />
              </div>
            </div>
          </div>
          <button type="submit" disabled={submitting} className="submit-btn">
            {submitting ? 'Creating...' : 'Create Subscription'}
          </button>
        </form>
      )}

      <div className="subs-table-wrap glass-card">
        {loading ? (
          <div className="subs-loading">Loading subscriptions...</div>
        ) : !data || data.subscriptions.length === 0 ? (
          <div className="subs-empty">
            <span className="subs-empty-icon">🔗</span>
            <p>No subscriptions yet. Click the button above to create one!</p>
          </div>
        ) : (
          <table className="subs-table">
            <thead>
              <tr>
                <th>Endpoint URL</th>
                <th>Event Types</th>
                <th>Status</th>
                <th>Created</th>
              </tr>
            </thead>
            <tbody>
              {data.subscriptions.map((sub) => (
                <tr key={sub.id} className="subs-row">
                  <td>
                    <span className="mono endpoint-cell" title={sub.endpoint_url}>
                      {sub.endpoint_url}
                    </span>
                  </td>
                  <td>
                    <div className="event-types-list">
                      {sub.event_types.map((t) => (
                        <span key={t} className="event-type-chip">{t}</span>
                      ))}
                    </div>
                  </td>
                  <td>
                    <span className={`sub-status ${sub.is_active ? 'sub-status--active' : 'sub-status--inactive'}`}>
                      {sub.is_active ? 'Active' : 'Inactive'}
                    </span>
                  </td>
                  <td className="text-muted" title={sub.created_at}>
                    {timeAgo(sub.created_at)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
