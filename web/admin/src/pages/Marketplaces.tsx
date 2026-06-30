import { useEffect, useState } from 'react'
import { apiGet, apiWrite } from '../api'
import type { MarketplaceStatus } from '../types'

const fieldLabels: Record<string, Record<string, string>> = {
  wb: { token: 'WB token' },
  ym: {
    api_key: 'API key',
    oauth_token: 'OAuth token',
    business_id: 'Business ID',
    campaign_id: 'Campaign ID',
  },
  ozon: {
    client_id: 'Client ID',
    api_key: 'API key',
  },
}

const defaultPublish: Record<string, boolean> = { wb: true, ym: true, ozon: false }

export default function Marketplaces() {
  const [items, setItems] = useState<MarketplaceStatus[]>([])
  const [drafts, setDrafts] = useState<Record<string, Record<string, string>>>({})
  const [busy, setBusy] = useState('')
  const [message, setMessage] = useState('')
  const [publish, setPublish] = useState<Record<string, boolean>>(defaultPublish)

  function load() {
    apiGet<{ marketplaces: MarketplaceStatus[] }>('/admin/api/marketplaces')
      .then((data) => setItems(data.marketplaces))
      .catch((err) => setMessage(err instanceof Error ? err.message : 'Запрос не выполнен'))
  }

  useEffect(load, [])

  useEffect(() => {
    apiGet<Record<string, string>>('/admin/api/settings').then((data) => {
      setPublish({
        wb: data['publishRepliesWb'] !== '' && data['publishRepliesWb'] !== undefined ? data['publishRepliesWb'] === 'true' : defaultPublish.wb,
        ym: data['publishRepliesYm'] !== '' && data['publishRepliesYm'] !== undefined ? data['publishRepliesYm'] === 'true' : defaultPublish.ym,
        ozon: data['publishRepliesOzon'] !== '' && data['publishRepliesOzon'] !== undefined ? data['publishRepliesOzon'] === 'true' : defaultPublish.ozon,
      })
    }).catch(() => {})
  }, [])

  async function togglePublish(mp: string, value: boolean) {
    setPublish((p) => ({ ...p, [mp]: value }))
    try {
      await apiWrite('PUT', '/admin/api/settings', { [`publish_replies_${mp}`]: value ? 'true' : 'false' })
    } catch (err) {
      setMessage(err instanceof Error ? err.message : 'Запрос не выполнен')
    }
  }

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

  async function refreshCatalog() {
    setBusy('catalog')
    setMessage('')
    try {
      const res = await apiWrite<{ products: number; articles: number }>('POST', '/admin/api/site-links/refresh')
      setMessage(`Каталог обновлён: товаров ${res.products}, артикулов с отзывами ${res.articles}`)
    } catch (err) {
      setMessage(err instanceof Error ? err.message : 'Запрос не выполнен')
    } finally {
      setBusy('')
    }
  }

  async function save(item: MarketplaceStatus, enabled = item.enabled) {
    setBusy(`save-${item.id}`)
    setMessage('')
    try {
      await apiWrite('PUT', `/admin/api/marketplaces/${item.id}/credentials`, {
        enabled,
        values: drafts[item.id] ?? {},
      })
      setDrafts({ ...drafts, [item.id]: {} })
      setMessage('Доступы сохранены')
      load()
    } catch (err) {
      setMessage(err instanceof Error ? err.message : 'Запрос не выполнен')
    } finally {
      setBusy('')
    }
  }

  function setDraft(id: string, key: string, value: string) {
    setDrafts({ ...drafts, [id]: { ...(drafts[id] ?? {}), [key]: value } })
  }

  return (
    <section className="stack">
      <div className="toolbar">
        <button onClick={() => sync()} disabled={busy !== ''}>
          Синхронизировать всё
        </button>
        <button className="secondary" onClick={refreshCatalog} disabled={busy !== ''} title="Перечитать sitemap магазина и обновить каталог товаров">
          Обновить каталог товаров
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
          {items.map((item) => {
            const labels = fieldLabels[item.id] ?? {}
            return (
              <div className="table-row grid-marketplaces" key={item.id}>
                <strong>{item.id}</strong>
                <label className="inline-check">
                  <input type="checkbox" checked={item.enabled} onChange={(e) => save(item, e.target.checked)} disabled={busy !== ''} />
                  <span className={item.enabled ? 'status-ok' : 'status-muted'}>{item.enabled ? 'да' : 'нет'}</span>
                </label>
                <span className={item.configured ? 'status-ok' : 'status-warn'}>
                  {item.configured ? 'настроены' : 'нет'}
                </span>
                <button className="secondary" onClick={() => sync(item.id)} disabled={busy !== '' || !item.enabled}>
                  Запуск
                </button>
                <div className="credential-grid">
                  {Object.entries(labels).map(([key, label]) => (
                    <label key={key}>
                      <span>{label}</span>
                      <input
                        value={drafts[item.id]?.[key] ?? ''}
                        onChange={(e) => setDraft(item.id, key, e.target.value)}
                        placeholder={item.fields?.[key] ? 'уже задан' : 'не задан'}
                        type={key.includes('token') || key.includes('key') ? 'password' : 'text'}
                      />
                    </label>
                  ))}
                  <button className="secondary" onClick={() => save(item)} disabled={busy !== ''}>
                    Сохранить
                  </button>
                </div>
                <label className="inline-check">
                  <input
                    type="checkbox"
                    checked={publish[item.id] ?? defaultPublish[item.id] ?? false}
                    onChange={(e) => togglePublish(item.id, e.target.checked)}
                    disabled={busy !== ''}
                  />
                  <span>Публиковать ответы на МП</span>
                </label>
              </div>
            )
          })}
        </div>
      </section>
    </section>
  )
}
