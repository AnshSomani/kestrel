import { Routes, Route, Navigate } from 'react-router-dom'
import Sidebar from './components/Sidebar'
import Overview from './pages/Overview'
import Events from './pages/Events'
import Subscriptions from './pages/Subscriptions'
import { Login } from './pages/Login'
import Users from './pages/Users'
import APIKeys from './pages/APIKeys'
import { useHealth } from './hooks/useApi'
import { useAuth } from './auth/AuthContext'
import './App.css'

export default function App() {
  const { data: health } = useHealth()
  const { isAuthenticated } = useAuth()
  const isLive = health?.status === 'ok'

  if (!isAuthenticated) {
    return (
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    )
  }

  return (
    <div className="app-layout">
      <Sidebar isLive={isLive} />
      <main className="app-main">
        <Routes>
          <Route path="/" element={<Overview />} />
          <Route path="/events" element={<Events />} />
          <Route path="/subscriptions" element={<Subscriptions />} />
          <Route path="/keys" element={<APIKeys />} />
          <Route path="/users" element={<Users />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </div>
  )
}
