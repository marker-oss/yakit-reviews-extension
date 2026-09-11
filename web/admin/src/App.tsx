import { useEffect, useMemo, useState } from 'react'
import { apiGet, apiWrite, clearCSRF } from './api'
import ToastHost from './components/ToastHost'
import Dashboard from './pages/Dashboard'
import Editor from './pages/Editor'
import Embed from './pages/Embed'
import Marketplaces from './pages/Marketplaces'
import Questions from './pages/Questions'
import Reviews from './pages/Reviews'
import Settings from './pages/Settings'
import Showcase from './pages/Showcase'
import Status from './pages/Status'
import Billing from './pages/Billing'
// OperatorPage is replaced by the hosted build (closed-source overlay):
// the open-source build ships a hidden no-op. The page itself renders only
// for owner sessions (server-gated /admin/api/saas/*).
let OperatorPage: (() => JSX.Element) | null = null
export function setOperatorPage(component: () => JSX.Element) {
  OperatorPage = component
}

type Mode = 'loading' | 'setup' | 'login' | 'authed'
type Route =
  | 'dashboard'
  | 'reviews'
  | 'questions'
  | 'status'
  | 'billing'
  | 'operator'
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
const ROUTES: Route[] = [
  'dashboard',
  'reviews',
  'questions',
  'status',
  'billing',
  'operator',
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
  if (route === 'status') return 'dashboard'
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
    case 'status':
      return 'Состояние'
    case 'billing':
      return 'Подписка'
    case 'operator':
      return 'Операторская панель'
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
  const [counts, setCounts] = useState<{ pendingReviews: number; pendingQuestions: number }>({
    pendingReviews: 0,
    pendingQuestions: 0,
  })
  // The operator tab appears only when the operator page component was
  // installed (hosted build) AND the session belongs to an owner.
  const [isOwner, setIsOwner] = useState(false)
  const hasOperator = isOwner && OperatorPage !== null
  useEffect(() => {
    apiGet<{ user_id: number; role: string }>('/admin/api/me')
      .then((me) => {
        setMode('authed')
        setIsOwner(me.role === 'owner')
      })
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

  useEffect(() => {
    if (mode !== 'authed') return
    apiGet<{ pendingReviews: number; pendingQuestions: number }>('/admin/api/counts')
      .then(setCounts)
      .catch(() => {})
  }, [mode, route])

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
      <>
        <main className="auth-screen">
          <p className="muted">Загрузка...</p>
        </main>
        <ToastHost />
      </>
    )
  }

  if (mode !== 'authed') {
    return (
      <>
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
        <ToastHost />
      </>
    )
  }

  return (
    <>
      <div className="view-shell">
        <header className="plat">
          <div className="plat-in">
            <a className="brand" href="#/dashboard">
              <span className="brand-mark">Виджет отзывов</span>
            </a>
            <span className="brand-tag">Админка</span>
            <nav className="hub-tabs" aria-label="Разделы">
              {[
                { route: 'dashboard' as Route, label: 'Сводка' },
                { route: 'reviews' as Route, label: 'Отзывы', count: counts.pendingReviews },
                { route: 'questions' as Route, label: 'Вопросы', count: counts.pendingQuestions },
                { route: 'status' as Route, label: 'Состояние' },
                { route: 'billing' as Route, label: 'Подписка' },
                { route: 'widget/showcase' as Route, label: 'Витрина' },
                { route: 'widget/editor' as Route, label: 'Редактор' },
                { route: 'widget/embed' as Route, label: 'Встраивание' },
                { route: 'settings/general' as Route, label: 'Общие' },
                { route: 'settings/marketplaces' as Route, label: 'Маркетплейсы' },
                ...(hasOperator ? [{ route: 'operator' as Route, label: 'SaaS' }] : []),
              ].map((item) => (
                <a
                  key={item.route}
                  className={`tab${route === item.route ? ' active' : ''}`}
                  href={`#/${item.route}`}
                >
                  {item.label}
                  {item.count ? <span className="nav-count">{item.count}</span> : null}
                </a>
              ))}
            </nav>
            <button className="plat-logout" onClick={logout}>
              Выйти
            </button>
          </div>
        </header>
        {error && <p className="error plat-error">{error}</p>}
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
          <h1>{title}</h1>
        </header>
        {route === 'dashboard' && <Dashboard />}
        {route === 'reviews' && <Reviews />}
        {route === 'questions' && <Questions />}
        {route === 'status' && <Status />}
        {route === 'billing' && <Billing />}
        {route === 'operator' && hasOperator && <OperatorPage />}
        {route === 'widget/showcase' && <Showcase />}
        {route === 'widget/editor' && <Editor />}
        {route === 'widget/embed' && <Embed />}
        {route === 'settings/general' && <Settings />}
        {route === 'settings/marketplaces' && <Marketplaces />}
      </div>
      <ToastHost />
    </>
  )
}
