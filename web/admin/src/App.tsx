import { useEffect, useMemo, useState } from 'react'
import { apiGet, apiWrite, clearCSRF } from './api'
import Dashboard from './pages/Dashboard'
import Marketplaces from './pages/Marketplaces'
import Reviews from './pages/Reviews'
import Showcase from './pages/Showcase'

type Mode = 'loading' | 'setup' | 'login' | 'authed'
type Page = 'dashboard' | 'reviews' | 'marketplaces' | 'showcase'

async function postAuth(path: string, body: unknown) {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const data = (await res.json().catch(() => ({ error: 'request failed' }))) as { error?: string }
    throw new Error(data.error ?? 'request failed')
  }
}

function currentPage(): Page {
  const raw = window.location.hash.replace(/^#\/?/, '')
  if (raw === 'reviews' || raw === 'marketplaces' || raw === 'showcase') return raw
  return 'dashboard'
}

export default function App() {
  const [mode, setMode] = useState<Mode>('loading')
  const [page, setPage] = useState<Page>(currentPage)
  const [login, setLogin] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    apiGet('/admin/api/me')
      .then(() => setMode('authed'))
      .catch(() => {
        fetch('/admin/api/setup-status')
          .then((s) => s.json())
          .then((data: { needs_setup: boolean }) => setMode(data.needs_setup ? 'setup' : 'login'))
          .catch(() => setMode('login'))
      })
  }, [])

  useEffect(() => {
    const onHash = () => setPage(currentPage())
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
  }, [])

  const title = useMemo(() => {
    if (page === 'reviews') return 'Reviews'
    if (page === 'marketplaces') return 'Marketplaces'
    if (page === 'showcase') return 'Showcase'
    return 'Dashboard'
  }, [page])

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    setError('')
    try {
      await postAuth(mode === 'setup' ? '/admin/api/setup' : '/admin/api/login', { login, password })
      setMode('authed')
      setPassword('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'request failed')
    }
  }

  async function logout() {
    setError('')
    try {
      await apiWrite('POST', '/admin/api/logout')
      clearCSRF()
      setMode('login')
      setPassword('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'request failed')
    }
  }

  if (mode === 'loading') {
    return (
      <main className="auth-screen">
        <p className="muted">Loading...</p>
      </main>
    )
  }

  if (mode !== 'authed') {
    return (
      <main className="auth-screen">
        <form className="auth-panel" onSubmit={submit}>
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

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div>
          <p className="eyebrow">Reviews</p>
          <h1>Admin</h1>
        </div>
        <nav>
          <a className={page === 'dashboard' ? 'active' : ''} href="#/dashboard">
            Dashboard
          </a>
          <a className={page === 'reviews' ? 'active' : ''} href="#/reviews">
            Reviews
          </a>
          <a className={page === 'marketplaces' ? 'active' : ''} href="#/marketplaces">
            Marketplaces
          </a>
          <a className={page === 'showcase' ? 'active' : ''} href="#/showcase">
            Showcase
          </a>
        </nav>
        <button className="secondary" onClick={logout}>
          Sign out
        </button>
        {error && <p className="error">{error}</p>}
      </aside>
      <main className="workspace">
        <header className="topbar">
          <h2>{title}</h2>
        </header>
        {page === 'dashboard' && <Dashboard />}
        {page === 'reviews' && <Reviews />}
        {page === 'marketplaces' && <Marketplaces />}
        {page === 'showcase' && <Showcase />}
      </main>
    </div>
  )
}
