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
      .catch((err) => setMessage(err instanceof Error ? err.message : 'Запрос не выполнен'))
  }, [])

  async function sync(marketplace?: string) {
    setBusy(marketplace ?? 'all')
    setMessage('')
    try {
      await apiWrite('POST', marketplace ? `/admin/api/sync?marketplace=${marketplace}` : '/admin/api/sync')
      setMessage('Синхронизация запущена')
    } catch (err) {
      setMessage(err instanceof Error ? err.message : 'Запрос не выполнен')
    } finally {
      setBusy('')
    }
  }

  return (
    <section className="stack">
      <div className="toolbar">
        <button onClick={() => sync()} disabled={busy !== ''}>
          Синхронизировать всё
        </button>
        {message && <p className="muted">{message}</p>}
      </div>
      <section className="panel">
        <div className="table">
          <div className="table-head grid-marketplaces">
            <span>Маркетплейс</span>
            <span>Включён</span>
            <span>Доступы</span>
            <span></span>
          </div>
          {items.map((item) => (
            <div className="table-row grid-marketplaces" key={item.id}>
              <strong>{item.id}</strong>
              <span className={item.enabled ? 'status-ok' : 'status-muted'}>{item.enabled ? 'да' : 'нет'}</span>
              <span className={item.configured ? 'status-ok' : 'status-warn'}>
                {item.configured ? 'настроены' : 'нет'}
              </span>
              <button className="secondary" onClick={() => sync(item.id)} disabled={busy !== '' || !item.enabled}>
                Запуск
              </button>
            </div>
          ))}
        </div>
      </section>
    </section>
  )
}
