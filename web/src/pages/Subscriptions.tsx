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
  const [editId, setEditId] = useState<string | null>(null)
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

  const handleEdit = (sub: any) => {
    setEditId(sub.id)
    setEndpoint(sub.endpoint_url)
    setSecret(sub.secret)
    setSelectedEvents(new Set(sub.event_types))
    
    // Add any custom events to availableEvents
    const nextAvailable = [...availableEvents]
    sub.event_types.forEach((t: string) => {
      if (!nextAvailable.includes(t)) {
        nextAvailable.push(t)
      }
    })
    setAvailableEvents(nextAvailable)
    setShowForm(true)
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  const handleCancel = () => {
    setShowForm(false)
    setEditId(null)
    setEndpoint('')
    setSecret('')
    setSelectedEvents(new Set(DEFAULT_EVENTS))
    setErrorMsg('')
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
      const url = editId 
        ? `${API_BASE}/api/subscriptions/${editId}`
        : `${API_BASE}/api/subscriptions`
      
      const res = await fetch(url, {
        method: editId ? 'PUT' : 'POST',
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
        throw new Error(editId ? 'Failed to update subscription' : 'Failed to create subscription')
      }
      handleCancel()
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
        <button className="create-sub-btn" onClick={() => showForm ? handleCancel() : setShowForm(true)}>
          {showForm ? 'Cancel' : '+ New Subscription'}
        </button>
      </div>

      {showForm && (
        <form className="glass-card create-sub-form" onSubmit={handleSubmit}>
          <h3>{editId ? 'Edit Subscription' : 'Create New Subscription'}</h3>
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
            {submitting ? 'Saving...' : (editId ? 'Save Changes' : 'Create Subscription')}
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
                <th>Actions</th>
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
                  <td>
                    <button 
                      className="pagination-btn"
                      style={{ padding: '0.25rem 0.75rem', fontSize: '0.85rem' }}
                      onClick={() => handleEdit(sub)}
                    >
                      Edit
                    </button>
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
