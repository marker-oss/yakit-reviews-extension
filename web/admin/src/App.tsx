import { useEffect, useMemo, useState } from 'react'
import { apiGet, apiWrite, clearCSRF } from './api'
import Dashboard from './pages/Dashboard'
import Editor from './pages/Editor'
import Embed from './pages/Embed'
import Marketplaces from './pages/Marketplaces'
import Questions from './pages/Questions'
import Reviews from './pages/Reviews'
import Settings from './pages/Settings'
import Showcase from './pages/Showcase'

type Mode = 'loading' | 'setup' | 'login' | 'authed'
type Route =
  | 'dashboard'
  | 'reviews'
  | 'questions'
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
  showcase: 'widget/showcase',
  editor: 'widget/editor',
  embed: 'widget/embed',
  settings: 'settings/general',
  marketplaces: 'settings/marketplaces',
}

const ROUTES: Route[] = [
  'dashboard',
  'reviews',
  'questions',
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

function routeSection(route: Route): 'dashboard' | 'reviews' | 'questions' | 'widget' | 'settings' {
  if (route === 'dashboard') return 'dashboard'
  if (route === 'reviews') return 'reviews'
  if (route === 'questions') return 'questions'
  return route.startsWith('widget/') ? 'widget' : 'settings'
}

function routeTitle(route: Route): string {
  switch (route) {
    case 'dashboard':
      return 'Сводка'
    case 'reviews':
      return 'Отзывы'
    case 'questions':
      return 'Вопросы'
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

type VersionInfo = {
  current: string
  latest: string
  updateAvailable: boolean
  releaseUrl: string
}

const UPDATE_DOCS_URL = 'https://github.com/marker-oss/yakit-reviews-extension#обслуживание'
const DISMISSED_KEY = 'reviews-update-dismissed'

export default function App() {
  const [mode, setMode] = useState<Mode>('loading')
  const [route, setRoute] = useState<Route>(currentRoute)
  const [login, setLogin] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [versionInfo, setVersionInfo] = useState<VersionInfo | null>(null)
  const [dismissedVersion, setDismissedVersion] = useState(() => localStorage.getItem(DISMISSED_KEY) ?? '')

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

  useEffect(() => {
    if (mode !== 'authed') return
    apiGet<VersionInfo>('/admin/api/version')
      .then(setVersionInfo)
      .catch(() => {})
  }, [mode])

  function dismissUpdate(version: string) {
    localStorage.setItem(DISMISSED_KEY, version)
    setDismissedVersion(version)
  }

  const showUpdateBanner =
    versionInfo !== null && versionInfo.updateAvailable && versionInfo.latest !== dismissedVersion

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
          <a className={route === 'dashboard' ? 'active' : ''} href="#/dashboard">
            Сводка
          </a>
          <a className={route === 'reviews' ? 'active' : ''} href="#/reviews">
            Отзывы
          </a>
          <a className={route === 'questions' ? 'active' : ''} href="#/questions">
            Вопросы
          </a>
          <div className="nav-group">
            <span className={`nav-group-label${routeSection(route) === 'widget' ? ' active' : ''}`}>
              Виджет
            </span>
            <a className={`nav-sub${route === 'widget/showcase' ? ' active' : ''}`} href="#/widget/showcase">
              Витрина
            </a>
            <a className={`nav-sub${route === 'widget/editor' ? ' active' : ''}`} href="#/widget/editor">
              Редактор
            </a>
            <a className={`nav-sub${route === 'widget/embed' ? ' active' : ''}`} href="#/widget/embed">
              Встраивание
            </a>
          </div>
          <div className="nav-group">
            <span className={`nav-group-label${routeSection(route) === 'settings' ? ' active' : ''}`}>
              Настройки
            </span>
            <a className={`nav-sub${route === 'settings/general' ? ' active' : ''}`} href="#/settings/general">
              Общие
            </a>
            <a className={`nav-sub${route === 'settings/marketplaces' ? ' active' : ''}`} href="#/settings/marketplaces">
              Маркетплейсы
            </a>
          </div>
        </nav>
        <button className="secondary" onClick={logout}>
          Выйти
        </button>
        {error && <p className="error">{error}</p>}
      </aside>
      <main className="workspace">
        {showUpdateBanner && versionInfo && (
          <div className="update-banner">
            <span>
              Доступна новая версия <strong>{versionInfo.latest}</strong> (у вас {versionInfo.current}).{' '}
              <a href={versionInfo.releaseUrl} target="_blank" rel="noreferrer">
                Что нового
              </a>{' '}
              ·{' '}
              <a href={UPDATE_DOCS_URL} target="_blank" rel="noreferrer">
                Как обновиться
              </a>
            </span>
            <button className="secondary" onClick={() => dismissUpdate(versionInfo.latest)}>
              Скрыть
            </button>
          </div>
        )}
        <header className="topbar">
          <h2>{title}</h2>
        </header>
        {route === 'dashboard' && <Dashboard />}
        {route === 'reviews' && <Reviews />}
        {route === 'questions' && <Questions />}
        {route === 'widget/showcase' && <Showcase />}
        {route === 'widget/editor' && <Editor />}
        {route === 'widget/embed' && <Embed />}
        {route === 'settings/general' && <Settings />}
        {route === 'settings/marketplaces' && <Marketplaces />}
      </main>
    </div>
  )
}
