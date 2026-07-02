import { useState, useEffect } from 'react'
import { timeAgo } from '../hooks/useApi'

interface User {
  id: string
  email: string
  role: string
  created_at: string
}

export default function Users() {
  const [users, setUsers] = useState<User[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    const fetchUsers = async () => {
      try {
        const token = localStorage.getItem('kestrel_access_token')
        const API_BASE = import.meta.env.VITE_API_URL || '';
        const res = await fetch(`${API_BASE}/api/users`, {
          headers: { 'Authorization': `Bearer ${token}` }
        })
        if (!res.ok) {
          if (res.status === 403) throw new Error('You do not have admin permissions to view this page.')
          throw new Error('Failed to fetch users')
        }
        const data = await res.json()
        setUsers(data.users || [])
      } catch (err: any) {
        setError(err.message)
      } finally {
        setLoading(false)
      }
    }
    fetchUsers()
  }, [])

  return (
    <div className="animate-fade-in" style={{ padding: '2rem' }}>
      <div style={{ marginBottom: '2rem' }}>
        <h1 style={{ margin: 0, fontSize: '2rem', fontWeight: 700, color: 'var(--text-primary)' }}>Users</h1>
        <p style={{ margin: '0.5rem 0 0 0', color: 'var(--text-muted)' }}>Registered accounts and roles</p>
      </div>

      {error && <div className="form-error" style={{ marginBottom: '1rem' }}>{error}</div>}

      <div className="glass-card" style={{ padding: '0', overflow: 'hidden' }}>
        {loading ? (
          <div style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-muted)' }}>Loading users...</div>
        ) : users.length === 0 ? (
          <div style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-muted)' }}>No users found.</div>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid var(--border-subtle)', background: 'rgba(0,0,0,0.2)' }}>
                <th style={{ padding: '16px 20px', fontSize: '12px', textTransform: 'uppercase', color: 'var(--text-muted)', fontWeight: 600 }}>Email</th>
                <th style={{ padding: '16px 20px', fontSize: '12px', textTransform: 'uppercase', color: 'var(--text-muted)', fontWeight: 600 }}>Role</th>
                <th style={{ padding: '16px 20px', fontSize: '12px', textTransform: 'uppercase', color: 'var(--text-muted)', fontWeight: 600 }}>ID</th>
                <th style={{ padding: '16px 20px', fontSize: '12px', textTransform: 'uppercase', color: 'var(--text-muted)', fontWeight: 600 }}>Created</th>
              </tr>
            </thead>
            <tbody>
              {users.map(u => (
                <tr key={u.id} style={{ borderBottom: '1px solid var(--border-subtle)' }}>
                  <td style={{ padding: '16px 20px', color: 'var(--text-primary)', fontWeight: 500 }}>{u.email}</td>
                  <td style={{ padding: '16px 20px' }}>
                    <span style={{
                      padding: '4px 10px',
                      borderRadius: '12px',
                      fontSize: '12px',
                      fontWeight: 600,
                      backgroundColor: u.role === 'admin' ? 'rgba(168, 85, 247, 0.15)' : 'rgba(6, 182, 212, 0.15)',
                      color: u.role === 'admin' ? 'var(--purple)' : 'var(--cyan)'
                    }}>
                      {u.role}
                    </span>
                  </td>
                  <td style={{ padding: '16px 20px', color: 'var(--text-muted)', fontSize: '13px', fontFamily: 'monospace' }}>{u.id.split('-')[0]}...</td>
                  <td style={{ padding: '16px 20px', color: 'var(--text-muted)', fontSize: '14px' }}>{timeAgo(u.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
