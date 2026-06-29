import { NavLink } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'
import './Sidebar.css'

const navItems = [
  { to: '/', icon: '📊', label: 'Overview', adminOnly: false },
  { to: '/events', icon: '⚡', label: 'Events', adminOnly: false },
  { to: '/subscriptions', icon: '🔗', label: 'Subscriptions', adminOnly: false },
  { to: '/keys', icon: '🔑', label: 'API Keys', adminOnly: false },
  { to: '/users', icon: '👥', label: 'Users', adminOnly: true },
]

interface SidebarProps {
  isLive: boolean
}

export default function Sidebar({ isLive }: SidebarProps) {
  const { logout, user } = useAuth()

  return (
    <aside className="sidebar">
      <div className="sidebar-header">
        <span className="sidebar-logo">🦅</span>
        <span className="sidebar-title">Kestrel</span>
      </div>

      <nav className="sidebar-nav">
        {navItems
          .filter(item => !item.adminOnly || user?.role === 'admin')
          .map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/'}
            className={({ isActive }) =>
              `sidebar-link ${isActive ? 'sidebar-link--active' : ''}`
            }
          >
            <span className="sidebar-link-icon">{item.icon}</span>
            <span className="sidebar-link-label">{item.label}</span>
          </NavLink>
        ))}
      </nav>

      <div className="sidebar-footer">
        <div className="sidebar-user-card">
          <div className="sidebar-user-info">
            <span className="sidebar-user-icon">🧑‍💻</span>
            <span className="sidebar-user-email">{user?.email?.split('@')[0] || 'admin'}</span>
          </div>
          <button className="sidebar-logout-btn" onClick={logout} title="Logout">
            <svg viewBox="0 0 24 24" width="16" height="16" stroke="currentColor" strokeWidth="2" fill="none" strokeLinecap="round" strokeLinejoin="round">
              <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"></path>
              <polyline points="16 17 21 12 16 7"></polyline>
              <line x1="21" y1="12" x2="9" y2="12"></line>
            </svg>
          </button>
        </div>
        <div className="sidebar-status">
          <span className={`status-dot ${isLive ? 'status-dot--live' : 'status-dot--offline'}`} />
          <span className="status-label">{isLive ? 'Live' : 'Offline'}</span>
        </div>
      </div>
    </aside>
  )
}
