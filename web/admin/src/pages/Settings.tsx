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
    </section>
  )
}
