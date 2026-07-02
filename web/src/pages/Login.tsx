import React, { useState } from 'react';
import { useAuth } from '../auth/AuthContext';
import './Login.css';

function decodeToken(token: string) {
  try {
    const base64Url = token.split('.')[1];
    const base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/');
    const payload = JSON.parse(window.atob(base64));
    return {
      id: payload.user_id,
      email: payload.email,
      role: payload.role || 'customer',
    };
  } catch {
    return null;
  }
}

export const Login: React.FC = () => {
  const [isSignup, setIsSignup] = useState(false);
  const [email, setEmail] = useState('admin@kestrel.local');
  const [password, setPassword] = useState('password');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const { login } = useAuth();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);

    try {
      const API_BASE = import.meta.env.VITE_API_URL || '';
      const endpoint = isSignup ? `${API_BASE}/api/auth/signup` : `${API_BASE}/api/auth/login`;
      const res = await fetch(endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password }),
      });

      const data = await res.json();

      if (!res.ok) {
        throw new Error(data.error || 'Authentication failed');
      }

      const user = decodeToken(data.access_token);
      if (user) {
        login(data.access_token, user);
      } else {
        throw new Error('Invalid token received');
      }
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="login-container">
      <div className="login-box animate-fade-in">
        <div className="login-header">
          <h1>🦅 Kestrel</h1>
          <p>Webhook Delivery Engine</p>
        </div>
        <form onSubmit={handleSubmit} className="login-form">
          {error && <div className="login-error">{error}</div>}
          <div className="form-group">
            <label>Email</label>
            <input 
              type="email" 
              value={email} 
              onChange={e => setEmail(e.target.value)} 
              required 
            />
          </div>
          <div className="form-group">
            <label>Password</label>
            <input 
              type="password" 
              value={password} 
              onChange={e => setPassword(e.target.value)} 
              required 
            />
          </div>
          <button type="submit" disabled={loading}>
            {loading ? 'Please wait...' : (isSignup ? 'Sign Up' : 'Sign In')}
          </button>
          
          <div style={{ textAlign: 'center', marginTop: '12px', fontSize: '13px' }}>
            <span style={{ color: 'var(--text-muted)' }}>
              {isSignup ? "Already have an account? " : "Don't have an account? "}
            </span>
            <button 
              type="button" 
              onClick={() => setIsSignup(!isSignup)}
              style={{ 
                background: 'none', border: 'none', color: 'var(--purple)', 
                cursor: 'pointer', padding: 0, fontWeight: 500,
                boxShadow: 'none', display: 'inline', marginTop: 0
              }}
            >
              {isSignup ? 'Log in' : 'Sign up'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
