import { useEffect, useState } from 'react'
import { apiGet } from '../api'
import type { DashboardData } from '../types'

export default function Dashboard() {
  const [data, setData] = useState<DashboardData | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    apiGet<DashboardData>('/admin/api/dashboard').then(setData).catch((err) => setError(String(err)))
  }, [])

  if (error) return <p className="error">{error}</p>
  if (!data) return <p className="muted">Loading...</p>

  return (
    <section className="stack">
      <div className="metrics">
        <div className="metric">
          <span>Total reviews</span>
          <strong>{data.total_reviews}</strong>
        </div>
        <div className="metric">
          <span>Average rating</span>
          <strong>{data.average_rating.toFixed(2)}</strong>
        </div>
      </div>
      <section className="panel">
        <h3>By marketplace</h3>
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
        <h3>Recent syncs</h3>
        <div className="rows">
          {data.recent_syncs.length === 0 && <p className="muted">No sync runs yet.</p>}
          {data.recent_syncs.map((run) => (
            <div className="row" key={run.ID}>
              <span>{run.Marketplace}</span>
              <span>{run.Status}</span>
              <span>{new Date(run.StartedAt).toLocaleString()}</span>
            </div>
          ))}
        </div>
      </section>
    </section>
  )
}
