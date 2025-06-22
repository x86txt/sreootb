'use client';

import React, { createContext, useContext, useState, useEffect, ReactNode } from 'react';

interface AuthContextType {
  isAuthenticated: boolean;
  apiKey: string | null;
  isLoading: boolean;
  login: (key: string) => Promise<boolean>;
  logout: () => void;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [apiKey, setApiKey] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    // Check for API key in local storage on initial load
    const storedKey = localStorage.getItem('apiKey');
    if (storedKey) {
      // Here you might want to validate the key against the backend
      // For now, we'll just assume it's valid if it exists
      setApiKey(storedKey);
    }
    setIsLoading(false);
  }, []);

  const login = async (key: string): Promise<boolean> => {
    // We'll add the backend validation later. For now, just assume it's good.
    const response = await fetch('/api/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ apiKey: key }),
    });
    if (response.ok) {
      setApiKey(key);
      localStorage.setItem('apiKey', key);
      // Set a cookie for middleware to read
      document.cookie = `apiKey=${key}; path=/; max-age=86400;`; // Expires in 1 day
      return true;
    }
    return false;
  };

  const logout = () => {
    setApiKey(null);
    localStorage.removeItem('apiKey');
    // Clear the cookie
    document.cookie = 'apiKey=; path=/; expires=Thu, 01 Jan 1970 00:00:00 GMT;';
  };

  const value = {
    isAuthenticated: !!apiKey,
    apiKey,
    isLoading,
    login,
    logout,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
} 