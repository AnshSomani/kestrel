import React, { createContext, useContext, useState, useEffect, ReactNode } from 'react';

interface User {
  id: string;
  email: string;
  role: string;
}

interface AuthContextType {
  token: string | null;
  user: User | null;
  login: (token: string, user: User) => void;
  logout: () => void;
  isAuthenticated: boolean;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

function decodeToken(token: string): User | null {
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

export const AuthProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const [token, setToken] = useState<string | null>(localStorage.getItem('kestrel_access_token'));
  const [user, setUser] = useState<User | null>(() => {
    const saved = localStorage.getItem('kestrel_user');
    if (!saved || saved === 'undefined') return null;
    try {
      return JSON.parse(saved);
    } catch {
      return null;
    }
  });

  const login = (newToken: string, newUser: User) => {
    setToken(newToken);
    setUser(newUser);
    localStorage.setItem('kestrel_access_token', newToken);
    localStorage.setItem('kestrel_user', JSON.stringify(newUser));
  };

  const logout = () => {
    setToken(null);
    setUser(null);
    localStorage.removeItem('kestrel_access_token');
    localStorage.removeItem('kestrel_user');
    fetch('/api/auth/logout', { method: 'POST' }).catch(() => {});
  };

  useEffect(() => {
    // Attempt silent refresh on mount if we have no token but maybe a cookie
    if (!token) {
      fetch('/api/auth/refresh', { method: 'POST' })
        .then(res => res.json())
        .then(data => {
          if (data.access_token) {
            const decoded = decodeToken(data.access_token);
            if (decoded) {
              login(data.access_token, decoded);
            }
          }
        })
        .catch(() => {});
    }

    const handleExpired = () => {
      setToken(null);
      setUser(null);
    };
    window.addEventListener('auth:expired', handleExpired);
    return () => window.removeEventListener('auth:expired', handleExpired);
  }, []);

  return (
    <AuthContext.Provider value={{ token, user, login, logout, isAuthenticated: !!token }}>
      {children}
    </AuthContext.Provider>
  );
};

export const useAuth = () => {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
};
