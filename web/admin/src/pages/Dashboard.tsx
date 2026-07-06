import { useEffect, useState } from 'react'
import { apiGet } from '../api'
import type { DashboardData } from '../types'

function syncStatus(value: string) {
  if (value === 'ok') return 'успешно'
  if (value === 'error') return 'ошибка'
  if (value === 'running') return 'выполняется'
  return value
}

export default function Dashboard() {
  const [data, setData] = useState<DashboardData | null>(null)
  const [error, setError] = useState('')
  const [diag, setDiag] = useState<{ checks: { id: string; level: string }[] } | null>(null)

  useEffect(() => {
    apiGet<DashboardData>('/admin/api/dashboard')
      .then(setData)
      .catch((err) => setError(err instanceof Error ? err.message : 'Запрос не выполнен'))
  }, [])

  useEffect(() => {
    apiGet<{ checks: { id: string; level: string }[] }>('/admin/api/diagnostics')
      .then(setDiag)
      .catch(() => {})
  }, [])

  if (error) return <p className="error">{error}</p>
  if (!data) return <p className="muted">Загрузка...</p>

  return (
    <section className="stack">
      {diag && (() => {
        const fails = diag.checks.filter((c) => c.level === 'fail').length
        const warns = diag.checks.filter((c) => c.level === 'warn').length
        const tone = fails > 0 ? 'fail' : warns > 0 ? 'warn' : 'ok'
        const label =
          tone === 'ok' ? 'Всё в порядке' : `${fails} проблем · ${warns} предупреждений`
        return (
          <a className={`health-strip health-${tone}`} href="#/status">
            <span>Состояние: {label}</span>
            <span className="muted">Подробнее →</span>
          </a>
        )
      })()}
      <div className="metrics">
        <div className="metric">
          <span>Всего отзывов</span>
          <strong>{data.total_reviews}</strong>
        </div>
        <div className="metric">
          <span>Показано на сайте</span>
          <strong>{data.visible_reviews}</strong>
        </div>
        <div className="metric">
          <span>Удалено</span>
          <strong>{data.deleted_reviews}</strong>
        </div>
        <div className="metric">
          <span>На модерации</span>
          <strong>{data.pending_reviews}</strong>
        </div>
        <div className="metric">
          <span>Средняя оценка (показанные)</span>
          <strong>{data.average_rating.toFixed(2)}</strong>
        </div>
      </div>
      <section className="panel">
        <h3>По маркетплейсам</h3>
        <div className="rows">
          {Object.entries(data.by_marketplace).map(([marketplace, count]) => (
            <div className="row" key={marketplace}>
              <span>{marketplace}</span>
              <strong>{count}</strong>
            </div>
          ))}
        </div>
      </section>
      <section className="panel">
        <h3>Последние синхронизации</h3>
        <div className="rows">
          {data.recent_syncs.length === 0 && <p className="muted">Синхронизаций пока не было.</p>}
          {data.recent_syncs.map((run) => (
            <div className="row" key={run.ID}>
              <span>{run.Marketplace}</span>
              <span>{syncStatus(run.Status)}</span>
              <span>{new Date(run.StartedAt).toLocaleString()}</span>
            </div>
          ))}
        </div>
      </section>
    </section>
  )
}
