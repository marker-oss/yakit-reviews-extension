import { useEffect, useState } from 'react'
import { apiGet, apiWrite } from '../api'
import type { MarketplaceStatus } from '../types'

export default function Marketplaces() {
  const [items, setItems] = useState<MarketplaceStatus[]>([])
  const [busy, setBusy] = useState('')
  const [message, setMessage] = useState('')

  useEffect(() => {
    apiGet<{ marketplaces: MarketplaceStatus[] }>('/admin/api/marketplaces')
      .then((data) => setItems(data.marketplaces))
      .catch((err) => setMessage(String(err)))
  }, [])

  async function sync(marketplace?: string) {
    setBusy(marketplace ?? 'all')
    setMessage('')
    try {
      await apiWrite('POST', marketplace ? `/admin/api/sync?marketplace=${marketplace}` : '/admin/api/sync')
      setMessage('Sync started')
    } catch (err) {
      setMessage(err instanceof Error ? err.message : 'request failed')
    } finally {
      setBusy('')
    }
  }

  return (
    <section className="stack">
      <div className="toolbar">
        <button onClick={() => sync()} disabled={busy !== ''}>
          Sync all
        </button>
        {message && <p className="muted">{message}</p>}
      </div>
      <section className="panel">
        <div className="table">
          <div className="table-head grid-marketplaces">
            <span>Marketplace</span>
            <span>Enabled</span>
            <span>Credentials</span>
            <span></span>
          </div>
          {items.map((item) => (
            <div className="table-row grid-marketplaces" key={item.id}>
              <strong>{item.id}</strong>
              <span className={item.enabled ? 'status-ok' : 'status-muted'}>{item.enabled ? 'enabled' : 'disabled'}</span>
              <span className={item.configured ? 'status-ok' : 'status-warn'}>
                {item.configured ? 'configured' : 'missing'}
              </span>
              <button className="secondary" onClick={() => sync(item.id)} disabled={busy !== '' || !item.enabled}>
                Sync
              </button>
            </div>
          ))}
        </div>
      </section>
    </section>
  )
}
