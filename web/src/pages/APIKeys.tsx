import { useState, useEffect } from 'react'
import { timeAgo } from '../hooks/useApi'

interface APIKey {
  id: string
  key?: string
  prefix: string
  created_at: string
}

export default function APIKeys() {
  const [keys, setKeys] = useState<APIKey[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [newKey, setNewKey] = useState<string | null>(null)
  const [generating, setGenerating] = useState(false)

  const fetchKeys = async () => {
    try {
      const token = localStorage.getItem('kestrel_access_token')
      const API_BASE = import.meta.env.VITE_API_URL || '';
      const res = await fetch(`${API_BASE}/api/keys`, {
        headers: { 'Authorization': `Bearer ${token}` }
      })
      if (!res.ok) throw new Error('Failed to fetch API keys')
      const data = await res.json()
      setKeys(data.keys || [])
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchKeys()
  }, [])

  const handleCreate = async () => {
    setGenerating(true)
    setError('')
    try {
      const token = localStorage.getItem('kestrel_access_token')
      const API_BASE = import.meta.env.VITE_API_URL || '';
      const res = await fetch(`${API_BASE}/api/keys`, {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${token}` }
      })
      if (!res.ok) throw new Error('Failed to generate key')
      const data = await res.json()
      setNewKey(data.key)
      setKeys([data, ...keys])
    } catch (err: any) {
      setError(err.message)
    } finally {
      setGenerating(false)
    }
  }

  const handleDelete = async (id: string) => {
    if (!confirm('Are you sure you want to revoke this API key? Any applications using it will immediately lose access.')) return
    
    try {
      const token = localStorage.getItem('kestrel_access_token')
      const API_BASE = import.meta.env.VITE_API_URL || '';
      const res = await fetch(`${API_BASE}/api/keys/${id}`, {
        method: 'DELETE',
        headers: { 'Authorization': `Bearer ${token}` }
      })
      if (!res.ok) throw new Error('Failed to revoke key')
      setKeys(keys.filter(k => k.id !== id))
    } catch (err: any) {
      alert(err.message)
    }
  }

  return (
    <div className="animate-fade-in" style={{ padding: '2rem' }}>
      <div style={{ marginBottom: '2rem', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <h1 style={{ margin: 0, fontSize: '2rem', fontWeight: 700, color: 'var(--text-primary)' }}>API Keys</h1>
          <p style={{ margin: '0.5rem 0 0 0', color: 'var(--text-muted)' }}>Manage your secret keys for API access</p>
        </div>
        <button 
          onClick={handleCreate} 
          disabled={generating}
          style={{
            background: 'var(--purple)',
            color: 'white',
            border: 'none',
            padding: '10px 20px',
            borderRadius: '8px',
            fontWeight: 600,
            cursor: 'pointer',
            transition: 'opacity 0.2s'
          }}
        >
          {generating ? 'Generating...' : '+ Generate New Key'}
        </button>
      </div>

      {error && <div className="form-error" style={{ marginBottom: '1rem' }}>{error}</div>}

      {newKey && (
        <div style={{ 
          background: 'rgba(16, 185, 129, 0.1)', 
          border: '1px solid rgba(16, 185, 129, 0.3)',
          borderRadius: '12px',
          padding: '20px',
          marginBottom: '2rem'
        }}>
          <h3 style={{ margin: '0 0 10px 0', color: 'var(--text-primary)' }}>Key Generated Successfully</h3>
          <p style={{ margin: '0 0 15px 0', color: 'var(--text-muted)' }}>
            Please copy your new API key now. For your security, it won't be shown again.
          </p>
          <div style={{
            display: 'flex',
            alignItems: 'center',
            gap: '10px',
            background: 'rgba(0,0,0,0.3)',
            padding: '12px 16px',
            borderRadius: '8px',
            fontFamily: 'monospace',
            color: '#10b981',
            fontSize: '16px',
            wordBreak: 'break-all'
          }}>
            {newKey}
          </div>
        </div>
      )}

      <div className="glass-card" style={{ padding: '0', overflow: 'hidden' }}>
        {loading ? (
          <div style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-muted)' }}>Loading keys...</div>
        ) : keys.length === 0 ? (
          <div style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-muted)' }}>You don't have any active API keys.</div>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid var(--border-subtle)', background: 'rgba(0,0,0,0.2)' }}>
                <th style={{ padding: '16px 20px', fontSize: '12px', textTransform: 'uppercase', color: 'var(--text-muted)', fontWeight: 600 }}>Key Prefix</th>
                <th style={{ padding: '16px 20px', fontSize: '12px', textTransform: 'uppercase', color: 'var(--text-muted)', fontWeight: 600 }}>Created</th>
                <th style={{ padding: '16px 20px', fontSize: '12px', textTransform: 'uppercase', color: 'var(--text-muted)', fontWeight: 600, textAlign: 'right' }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {keys.map(k => (
                <tr key={k.id} style={{ borderBottom: '1px solid var(--border-subtle)' }}>
                  <td style={{ padding: '16px 20px', color: 'var(--text-primary)', fontFamily: 'monospace', fontSize: '14px' }}>
                    {k.prefix}
                  </td>
                  <td style={{ padding: '16px 20px', color: 'var(--text-muted)', fontSize: '14px' }}>
                    {timeAgo(k.created_at)}
                  </td>
                  <td style={{ padding: '16px 20px', textAlign: 'right' }}>
                    <button 
                      onClick={() => handleDelete(k.id)}
                      style={{
                        background: 'rgba(239, 68, 68, 0.1)',
                        color: 'var(--red)',
                        border: 'none',
                        padding: '6px 12px',
                        borderRadius: '6px',
                        fontSize: '12px',
                        fontWeight: 600,
                        cursor: 'pointer'
                      }}
                    >
                      Revoke
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
