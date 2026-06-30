import { useEffect, useState } from 'react'
import { apiGet, apiWrite } from '../api'

interface SettingsResponse {
  agreementUrl: string
  shopOrigin: string
  sitemapUrl: string
}

export default function Settings() {
  const [agreementUrl, setAgreementUrl] = useState('')
  const [shopOrigin, setShopOrigin] = useState('')
  const [sitemapUrl, setSitemapUrl] = useState('')
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')

  // DSR (152-ФЗ) state
  const [dsrEmail, setDsrEmail] = useState('')
  const [dsrResult, setDsrResult] = useState<{ reviews: unknown[] } | null>(null)
  const [dsrMsg, setDsrMsg] = useState('')
  const [dsrError, setDsrError] = useState('')
  const [dsrBusy, setDsrBusy] = useState(false)

  useEffect(() => {
    apiGet<SettingsResponse>('/admin/api/settings')
      .then((data) => {
        setAgreementUrl(data.agreementUrl ?? '')
        setShopOrigin(data.shopOrigin ?? '')
        setSitemapUrl(data.sitemapUrl ?? '')
      })
      .catch((err) => setError(err instanceof Error ? err.message : 'Запрос не выполнен'))
  }, [])

  async function save(event: React.FormEvent) {
    event.preventDefault()
    setBusy(true)
    setMessage('')
    setError('')
    try {
      const data = await apiWrite<SettingsResponse>('PUT', '/admin/api/settings', {
        agreementUrl: agreementUrl.trim(),
        shopOrigin: shopOrigin.trim(),
        sitemapUrl: sitemapUrl.trim(),
      })
      setAgreementUrl(data.agreementUrl ?? '')
      setShopOrigin(data.shopOrigin ?? '')
      setSitemapUrl(data.sitemapUrl ?? '')
      setMessage('Сохранено')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Запрос не выполнен')
    } finally {
      setBusy(false)
    }
  }

  async function dsrLookup() {
    setDsrMsg('')
    setDsrError('')
    setDsrResult(null)
    setDsrBusy(true)
    try {
      const data = await apiGet<{ reviews: unknown[] }>(
        `/admin/api/dsr/lookup?email=${encodeURIComponent(dsrEmail)}`,
      )
      setDsrResult(data)
      setDsrMsg(`Найдено отзывов: ${data.reviews.length}`)
    } catch (e) {
      setDsrError(e instanceof Error ? e.message : 'Ошибка')
    } finally {
      setDsrBusy(false)
    }
  }

  async function dsrDelete() {
    if (
      !window.confirm(
        'Удалить все данные этого субъекта без возможности восстановления?',
      )
    )
      return
    if (!window.confirm(`Подтвердите: безвозвратно удалить данные для "${dsrEmail}"?`)) return
    setDsrBusy(true)
    setDsrMsg('')
    setDsrError('')
    try {
      const r = await apiWrite<{ deleted: number }>('POST', '/admin/api/dsr/delete', {
        email: dsrEmail,
      })
      setDsrMsg(`Удалено: ${r.deleted}`)
      setDsrResult(null)
    } catch (e) {
      setDsrError(e instanceof Error ? e.message : 'Ошибка')
    } finally {
      setDsrBusy(false)
    }
  }

  return (
    <section className="stack">
      <section className="panel">
        <form className="stack" onSubmit={save}>
          <h3>Форма отзыва на сайте</h3>
          <p className="muted">
            Ссылка на страницу с согласием на обработку персональных данных. Показывается под
            формой отзыва рядом с галочкой согласия. Оставьте пустым, чтобы убрать ссылку.
          </p>
          <label className="stack">
            <span>URL страницы согласия / пользовательского соглашения</span>
            <input
              type="url"
              value={agreementUrl}
              onChange={(e) => setAgreementUrl(e.target.value)}
              placeholder="https://ваш-магазин.ру/personal-data-consent"
            />
          </label>

          <h3>Каталог товаров</h3>
          <p className="muted">
            Источник для кнопки «Обновить каталог товаров» на странице «Маркетплейсы». Каталог
            строится обходом sitemap магазина. Укажите адрес магазина — sitemap возьмётся как
            <code> /sitemap.xml</code>, либо задайте точный адрес sitemap, если он по другому пути.
          </p>
          <label className="stack">
            <span>Адрес магазина (origin)</span>
            <input
              type="url"
              value={shopOrigin}
              onChange={(e) => setShopOrigin(e.target.value)}
              placeholder="https://ваш-магазин.ру"
            />
          </label>
          <label className="stack">
            <span>Адрес sitemap (необязательно — переопределяет адрес магазина)</span>
            <input
              type="url"
              value={sitemapUrl}
              onChange={(e) => setSitemapUrl(e.target.value)}
              placeholder="https://ваш-магазин.ру/sitemap.xml"
            />
          </label>

          <div className="toolbar">
            <button type="submit" disabled={busy}>
              {busy ? 'Сохраняем' : 'Сохранить'}
            </button>
            {message && <p className="muted">{message}</p>}
            {error && <p className="error">{error}</p>}
          </div>
        </form>
      </section>

      <section className="panel">
        <div className="stack">
          <h3>Запросы субъектов (152-ФЗ)</h3>
          <p className="muted">
            Поиск, выгрузка и удаление персональных данных по запросу субъекта. Укажите email,
            использованный при отправке отзыва на сайте.
          </p>
          <label className="stack">
            <span>Email субъекта</span>
            <input
              type="email"
              value={dsrEmail}
              onChange={(e) => setDsrEmail(e.target.value)}
              placeholder="subject@example.com"
            />
          </label>
          <div className="toolbar">
            <button type="button" onClick={dsrLookup} disabled={dsrBusy || !dsrEmail.trim()}>
              Найти
            </button>
            {dsrResult !== null && (
              <a href={`/admin/api/dsr/export?email=${encodeURIComponent(dsrEmail)}`}>
                Скачать выгрузку
              </a>
            )}
            {dsrResult !== null && dsrResult.reviews.length > 0 && (
              <button
                type="button"
                onClick={dsrDelete}
                disabled={dsrBusy}
                style={{ color: 'var(--color-danger, #c0392b)' }}
              >
                Удалить все данные
              </button>
            )}
          </div>
          {dsrMsg && <p className="muted">{dsrMsg}</p>}
          {dsrError && <p className="error">{dsrError}</p>}
          <p className="muted">
            Для отзывов с маркетплейсов удаляется только наша копия. Оригинал на WB / Ozon /
            Яндекс Маркет удаляется через сам маркетплейс.
          </p>
        </div>
      </section>
    </section>
  )
}
