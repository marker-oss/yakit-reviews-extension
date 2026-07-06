import { useEffect, useState } from 'react'
import { apiGet, apiWrite } from '../api'
import { toast } from '../toast'
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

type CatalogStatus = {
  state: 'idle' | 'running' | 'done' | 'error'
  total: number
  crawled: number
  products: number
  articles: number
  error?: string
}

function catalogStatusText(status: CatalogStatus | null): string {
  if (!status) return ''
  switch (status.state) {
    case 'running':
      return status.total > 0
        ? `Обновление каталога: ${status.crawled} из ${status.total} новых товаров…`
        : 'Обновление каталога: читаем sitemap…'
    case 'done':
      return `Каталог обновлён: товаров ${status.products}, артикулов с отзывами ${status.articles}`
    case 'error':
      return `Каталог: ${status.error ?? 'обновление не удалось'}`
    default:
      return ''
  }
}

export default function Marketplaces() {
  const [items, setItems] = useState<MarketplaceStatus[]>([])
  const [drafts, setDrafts] = useState<Record<string, Record<string, string>>>({})
  const [busy, setBusy] = useState('')
  const [publish, setPublish] = useState<Record<string, boolean>>(defaultPublish)
  const [catalog, setCatalog] = useState<CatalogStatus | null>(null)
  const [catalogPollEpoch, setCatalogPollEpoch] = useState(0)

  // Poll the background catalog-refresh job while it runs; also picks up a job
  // already started earlier (page reload, another tab).
  useEffect(() => {
    let cancelled = false
    let timer: number | undefined
    const tick = () => {
      apiGet<CatalogStatus>('/admin/api/site-links/refresh')
        .then((status) => {
          if (cancelled) return
          setCatalog(status)
          if (status.state === 'running') timer = window.setTimeout(tick, 2000)
        })
        .catch(() => {})
    }
    tick()
    return () => {
      cancelled = true
      if (timer !== undefined) window.clearTimeout(timer)
    }
  }, [catalogPollEpoch])

  function load() {
    apiGet<{ marketplaces: MarketplaceStatus[] }>('/admin/api/marketplaces')
      .then((data) => setItems(data.marketplaces))
      .catch((err) => toast.error(err instanceof Error ? err.message : 'Запрос не выполнен'))
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
      toast.error(err instanceof Error ? err.message : 'Запрос не выполнен')
    }
  }

  async function sync(marketplace?: string) {
    setBusy(marketplace ?? 'all')
    try {
      await apiWrite('POST', marketplace ? `/admin/api/sync?marketplace=${marketplace}` : '/admin/api/sync')
      toast.success('Синхронизация запущена')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Запрос не выполнен')
    } finally {
      setBusy('')
    }
  }

  async function refreshCatalog(full = false) {
    setBusy('catalog')
    try {
      await apiWrite<CatalogStatus>('POST', `/admin/api/site-links/refresh${full ? '?full=1' : ''}`)
    } catch (err) {
      // 409 «уже идёт» тоже прилетает сюда — тогда просто продолжаем опрашивать.
      const status = await apiGet<CatalogStatus>('/admin/api/site-links/refresh').catch(() => null)
      if (!status || status.state !== 'running') {
        toast.error(err instanceof Error ? err.message : 'Запрос не выполнен')
        setBusy('')
        return
      }
    }
    setBusy('')
    setCatalogPollEpoch((n) => n + 1)
  }

  async function save(item: MarketplaceStatus, enabled = item.enabled) {
    setBusy(`save-${item.id}`)
    try {
      await apiWrite('PUT', `/admin/api/marketplaces/${item.id}/credentials`, {
        enabled,
        values: drafts[item.id] ?? {},
      })
      setDrafts({ ...drafts, [item.id]: {} })
      toast.success('Доступы сохранены. Запустите синхронизацию, чтобы подтянуть отзывы', {
        label: 'Синхронизировать',
        onClick: () => sync(item.id),
      })
      load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Запрос не выполнен')
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
        <button
          className="secondary"
          onClick={() => refreshCatalog()}
          disabled={busy !== '' || catalog?.state === 'running'}
          title="Перечитать sitemap магазина и добавить в каталог новые товары (уже известные не перечитываются)"
        >
          Обновить каталог товаров
        </button>
        <button
          className="secondary"
          onClick={() => refreshCatalog(true)}
          disabled={busy !== '' || catalog?.state === 'running'}
          title="Заново обойти все страницы товаров — долго, нужно только если поменялись артикулы или названия"
        >
          Пересканировать полностью
        </button>
        {catalogStatusText(catalog) && <p className="muted">{catalogStatusText(catalog)}</p>}
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
                {item.warning && <p className="status-warn">{item.warning}</p>}
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
