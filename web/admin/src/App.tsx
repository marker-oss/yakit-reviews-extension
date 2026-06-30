import { useEffect, useMemo, useState } from 'react'
import { apiGet, apiWrite, clearCSRF } from './api'
import Dashboard from './pages/Dashboard'
import Editor from './pages/Editor'
import Embed from './pages/Embed'
import Marketplaces from './pages/Marketplaces'
import Reviews from './pages/Reviews'
import Settings from './pages/Settings'
import Showcase from './pages/Showcase'

type Mode = 'loading' | 'setup' | 'login' | 'authed'
type Route =
  | 'dashboard'
  | 'reviews'
  | 'widget/showcase'
  | 'widget/editor'
  | 'widget/embed'
  | 'settings/general'
  | 'settings/marketplaces'

async function postAuth(path: string, body: unknown) {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const data = (await res.json().catch(() => ({ error: 'Запрос не выполнен' }))) as { error?: string }
    throw new Error(data.error ?? 'Запрос не выполнен')
  }
}

const LEGACY_ROUTES: Record<string, Route> = {
  '': 'dashboard',
  dashboard: 'dashboard',
  reviews: 'reviews',
  showcase: 'widget/showcase',
  editor: 'widget/editor',
  embed: 'widget/embed',
  settings: 'settings/general',
  marketplaces: 'settings/marketplaces',
}

const ROUTES: Route[] = [
  'dashboard',
  'reviews',
  'widget/showcase',
  'widget/editor',
  'widget/embed',
  'settings/general',
  'settings/marketplaces',
]

function currentRoute(): Route {
  const raw = window.location.hash.replace(/^#\/?/, '')
  if ((ROUTES as string[]).includes(raw)) return raw as Route
  if (raw in LEGACY_ROUTES) return LEGACY_ROUTES[raw]
  return 'dashboard'
}

function routeSection(route: Route): 'dashboard' | 'reviews' | 'widget' | 'settings' {
  if (route === 'dashboard') return 'dashboard'
  if (route === 'reviews') return 'reviews'
  return route.startsWith('widget/') ? 'widget' : 'settings'
}

function routeTitle(route: Route): string {
  switch (route) {
    case 'reviews':
      return 'Отзывы'
    case 'widget/showcase':
      return 'Виджет · Витрина'
    case 'widget/editor':
      return 'Виджет · Редактор'
    case 'widget/embed':
      return 'Виджет · Встраивание'
    case 'settings/general':
      return 'Настройки'
    case 'settings/marketplaces':
      return 'Настройки · Маркетплейсы'
    default:
      return 'Сводка'
  }
}

function authError(message: string) {
  if (message === 'authentication required') return 'Требуется вход в админку'
  if (message === 'invalid login or password') return 'Неверный логин или пароль'
  return message || 'Запрос не выполнен'
}

export default function App() {
  const [mode, setMode] = useState<Mode>('loading')
  const [route, setRoute] = useState<Route>(currentRoute)
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
    const onHash = () => setRoute(currentRoute())
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
  }, [])

  const title = useMemo(() => routeTitle(route), [route])

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    setError('')
    try {
      await postAuth(mode === 'setup' ? '/admin/api/setup' : '/admin/api/login', { login, password })
      setMode('authed')
      setPassword('')
    } catch (err) {
      setError(err instanceof Error ? authError(err.message) : 'Запрос не выполнен')
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
      setError(err instanceof Error ? authError(err.message) : 'Запрос не выполнен')
    }
  }

  if (mode === 'loading') {
    return (
      <main className="auth-screen">
        <p className="muted">Загрузка...</p>
      </main>
    )
  }

  if (mode !== 'authed') {
    return (
      <main className="auth-screen">
        <form className="auth-panel" onSubmit={submit}>
          <p className="eyebrow">{mode === 'setup' ? 'Первый запуск' : 'Вход'}</p>
          <h1>{mode === 'setup' ? 'Создать администратора' : 'Войти'}</h1>
          <label>
            <span>Логин</span>
            <input value={login} onChange={(e) => setLogin(e.target.value)} autoComplete="username" />
          </label>
          <label>
            <span>Пароль</span>
            <input
              value={password}
              type="password"
              onChange={(e) => setPassword(e.target.value)}
              autoComplete={mode === 'setup' ? 'new-password' : 'current-password'}
            />
          </label>
          <button type="submit">{mode === 'setup' ? 'Создать' : 'Войти'}</button>
          {error && <p className="error">{error}</p>}
        </form>
      </main>
    )
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div>
          <p className="eyebrow">Отзывы</p>
          <h1>Админка</h1>
        </div>
        <nav>
          <a className={page === 'dashboard' ? 'active' : ''} href="#/dashboard">
            Сводка
          </a>
          <a className={page === 'reviews' ? 'active' : ''} href="#/reviews">
            Отзывы
          </a>
          <a className={page === 'marketplaces' ? 'active' : ''} href="#/marketplaces">
            Маркетплейсы
          </a>
          <a className={page === 'showcase' ? 'active' : ''} href="#/showcase">
            Витрина
          </a>
          <a className={page === 'editor' ? 'active' : ''} href="#/editor">
            Редактор
          </a>
          <a className={page === 'embed' ? 'active' : ''} href="#/embed">
            Встраивание
          </a>
          <a className={page === 'settings' ? 'active' : ''} href="#/settings">
            Настройки
          </a>
        </nav>
        <button className="secondary" onClick={logout}>
          Выйти
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
        {page === 'editor' && <Editor />}
        {page === 'embed' && <Embed />}
        {page === 'settings' && <Settings />}
      </main>
    </div>
  )
}
