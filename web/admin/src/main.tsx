import React, { useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import './styles.css'

type Mode = 'loading' | 'setup' | 'login' | 'authed'

async function post(path: string, body: unknown) {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    let message = 'request failed'
    try {
      const data = (await res.json()) as { error?: string }
      message = data.error ?? message
    } catch {
      // Keep the generic message.
    }
    throw new Error(message)
  }
}

function App() {
  const [mode, setMode] = useState<Mode>('loading')
  const [login, setLogin] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    fetch('/admin/api/me').then((res) => {
      if (res.ok) {
        setMode('authed')
        return
      }
      fetch('/admin/api/setup-status')
        .then((s) => s.json())
        .then((data: { needs_setup: boolean }) => setMode(data.needs_setup ? 'setup' : 'login'))
        .catch(() => setMode('login'))
    })
  }, [])

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    setError('')
    try {
      await post(mode === 'setup' ? '/admin/api/setup' : '/admin/api/login', { login, password })
      setMode('authed')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'request failed')
    }
  }

  async function logout() {
    setError('')
    try {
      const csrf = (await fetch('/admin/api/csrf').then((r) => r.json())) as { csrf_token: string }
      await fetch('/admin/api/logout', {
        method: 'POST',
        headers: { 'X-CSRF-Token': csrf.csrf_token },
      })
      setMode('login')
      setPassword('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'request failed')
    }
  }

  if (mode === 'loading') {
    return (
      <main className="shell">
        <p>Loading...</p>
      </main>
    )
  }

  if (mode === 'authed') {
    return (
      <main className="shell">
        <section className="panel">
          <div>
            <p className="eyebrow">Reviews</p>
            <h1>Admin</h1>
            <p className="muted">Authentication is ready. Dashboard and moderation arrive in the next stage.</p>
          </div>
          <button onClick={logout}>Sign out</button>
          {error && <p className="error">{error}</p>}
        </section>
      </main>
    )
  }

  return (
    <main className="shell">
      <form className="panel auth" onSubmit={submit}>
        <p className="eyebrow">{mode === 'setup' ? 'First run' : 'Welcome back'}</p>
        <h1>{mode === 'setup' ? 'Create admin' : 'Sign in'}</h1>
        <label>
          <span>Login</span>
          <input value={login} onChange={(e) => setLogin(e.target.value)} autoComplete="username" />
        </label>
        <label>
          <span>Password</span>
          <input
            value={password}
            type="password"
            onChange={(e) => setPassword(e.target.value)}
            autoComplete={mode === 'setup' ? 'new-password' : 'current-password'}
          />
        </label>
        <button type="submit">{mode === 'setup' ? 'Create' : 'Sign in'}</button>
        {error && <p className="error">{error}</p>}
      </form>
    </main>
  )
}

createRoot(document.getElementById('root')!).render(<App />)
